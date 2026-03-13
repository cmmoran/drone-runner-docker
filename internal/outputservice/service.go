package outputservice

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/drone-runners/drone-runner-docker/internal/outputproto"
	"github.com/drone-runners/drone-runner-docker/internal/stepoutput"
)

const DefaultTTL = 72 * time.Hour

type StepIdentity struct {
	PipelineID string
	Step       string
}

type PipelineOutputs struct {
	mu     sync.RWMutex
	byStep map[string]map[string]string
}

type Service struct {
	mu        sync.RWMutex
	pipelines map[string]*ttlEntry[*PipelineOutputs]
	tokens    map[string]*ttlEntry[StepIdentity]
	ttl       time.Duration
	now       func() time.Time
	stopCh    chan struct{}
}

type ttlEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func New(ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	svc := &Service{
		pipelines: map[string]*ttlEntry[*PipelineOutputs]{},
		tokens:    map[string]*ttlEntry[StepIdentity]{},
		ttl:       ttl,
		now:       time.Now,
		stopCh:    make(chan struct{}),
	}
	go svc.janitor()
	return svc
}

func (s *Service) Close() {
	close(s.stopCh)
}

func (s *Service) RegisterPipeline(pipelineID string, stepTokens map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt := s.now().Add(s.ttl)
	s.pipelines[pipelineID] = &ttlEntry[*PipelineOutputs]{
		value:     &PipelineOutputs{byStep: map[string]map[string]string{}},
		expiresAt: expiresAt,
	}
	for step, token := range stepTokens {
		s.tokens[token] = &ttlEntry[StepIdentity]{
			value:     StepIdentity{PipelineID: pipelineID, Step: step},
			expiresAt: expiresAt,
		}
	}
}

func (s *Service) DeletePipeline(pipelineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pipelines, pipelineID)
	for token, entry := range s.tokens {
		if entry.value.PipelineID == pipelineID {
			delete(s.tokens, token)
		}
	}
}

func (s *Service) Apply(token string, ops []outputproto.OutputOp) error {
	s.mu.Lock()
	tokenEntry, ok := s.tokens[token]
	if !ok || s.expired(tokenEntry.expiresAt) {
		if ok {
			delete(s.tokens, token)
		}
		s.mu.Unlock()
		return fmt.Errorf("invalid output token")
	}
	pipelineEntry, ok := s.pipelines[tokenEntry.value.PipelineID]
	if !ok || s.expired(pipelineEntry.expiresAt) {
		if ok {
			delete(s.pipelines, tokenEntry.value.PipelineID)
		}
		s.mu.Unlock()
		return fmt.Errorf("unknown output pipeline")
	}
	expiresAt := s.now().Add(s.ttl)
	tokenEntry.expiresAt = expiresAt
	pipelineEntry.expiresAt = expiresAt
	pipeline := pipelineEntry.value
	s.mu.Unlock()

	pipeline.mu.Lock()
	defer pipeline.mu.Unlock()
	values := pipeline.byStep[tokenEntry.value.Step]
	if values == nil {
		values = map[string]string{}
		pipeline.byStep[tokenEntry.value.Step] = values
	}
	for _, op := range ops {
		if err := op.Validate(); err != nil {
			return err
		}
		switch op.Op {
		case outputproto.OpSet:
			values[op.Key] = *op.Value
		case outputproto.OpUnset:
			delete(values, op.Key)
			prefix := op.Key + "."
			for key := range values {
				if key == op.Key || strings.HasPrefix(key, prefix) {
					delete(values, key)
				}
			}
		}
	}
	return nil
}

func (s *Service) Resolve(pipelineID, producer, key string) (string, bool, error) {
	s.mu.RLock()
	entry, ok := s.pipelines[pipelineID]
	if !ok || s.expired(entry.expiresAt) {
		s.mu.RUnlock()
		return "", false, nil
	}
	pipeline := entry.value
	s.mu.RUnlock()

	pipeline.mu.RLock()
	values := pipeline.byStep[producer]
	pipeline.mu.RUnlock()
	if len(values) == 0 {
		return "", false, nil
	}
	return stepoutput.ResolveValue(values, key)
}

func (s *Service) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.prune()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Service) prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for pipelineID, entry := range s.pipelines {
		if now.After(entry.expiresAt) {
			delete(s.pipelines, pipelineID)
		}
	}
	for token, entry := range s.tokens {
		if now.After(entry.expiresAt) {
			delete(s.tokens, token)
		}
	}
}

func (s *Service) expired(deadline time.Time) bool {
	return s.now().After(deadline)
}
