package outputexec

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/drone-runners/drone-runner-docker/engine"
	"github.com/drone-runners/drone-runner-docker/internal/outputservice"
	"github.com/drone-runners/drone-runner-docker/internal/stepoutput"

	"github.com/drone/drone-go/drone"
	"github.com/drone/runner-go/environ"
	"github.com/drone/runner-go/livelog/extractor"
	"github.com/drone/runner-go/logger"
	"github.com/drone/runner-go/pipeline"
	runtime "github.com/drone/runner-go/pipeline/runtime"

	"github.com/hashicorp/go-multierror"
	"github.com/natessilva/dag"
	"golang.org/x/sync/semaphore"
)

var noContext = context.Background()

type Execer struct {
	engine   runtime.Engine
	reporter pipeline.Reporter
	streamer pipeline.Streamer
	uploader pipeline.Uploader
	sem      *semaphore.Weighted
	service  *outputservice.Service

	mu      sync.RWMutex
	outputs map[string]map[string]string
}

func New(
	reporter pipeline.Reporter,
	streamer pipeline.Streamer,
	uploader pipeline.Uploader,
	engine runtime.Engine,
	threads int64,
	service *outputservice.Service,
) *Execer {
	exec := &Execer{
		reporter: reporter,
		streamer: streamer,
		engine:   engine,
		uploader: uploader,
		service:  service,
		outputs:  map[string]map[string]string{},
	}
	if threads > 0 {
		exec.sem = semaphore.NewWeighted(threads)
	}
	return exec
}

func (e *Execer) Exec(ctx context.Context, spec runtime.Spec, state *pipeline.State) error {
	log := logger.FromContext(ctx)
	defer func() {
		log.Debugln("destroying the pipeline environment")
		if engineSpec, ok := spec.(*engine.Spec); ok {
			if engineSpec.OutputCloser != nil {
				_ = engineSpec.OutputCloser.Close()
			}
			if e.service != nil && engineSpec.PipelineID != "" {
				e.service.DeletePipeline(engineSpec.PipelineID)
			}
		}
		if err := e.engine.Destroy(noContext, spec); err != nil {
			log.WithError(err).Debugln("cannot destroy the pipeline environment")
		}
		if engineSpec, ok := spec.(*engine.Spec); ok && engineSpec.OutputDir != "" {
			_ = os.RemoveAll(engineSpec.OutputDir)
		}
	}()

	if err := e.engine.Setup(noContext, spec); err != nil {
		state.FailAll(err)
		return e.reporter.ReportStage(noContext, state)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var d dag.Runner
	for i := 0; i < spec.StepLen(); i++ {
		step := spec.StepAt(i)
		d.AddVertex(step.GetName(), func() error {
			err := e.exec(ctx, state, spec, step)
			if step.GetErrPolicy() == runtime.ErrFailFast {
				stepState := state.Find(step.GetName())
				state.Lock()
				exit := stepState.ExitCode
				state.Unlock()
				if exit > 0 {
					cancel()
				}
			}
			return err
		})
	}
	for i := 0; i < spec.StepLen(); i++ {
		step := spec.StepAt(i)
		for _, dep := range step.GetDependencies() {
			d.AddEdge(dep, step.GetName())
		}
	}

	var result error
	if err := d.Run(); err != nil {
		result = multierror.Append(result, err)
		if !state.Failed() {
			state.FailAll(err)
		}
	}
	state.FinishAll()
	if err := e.reporter.ReportStage(noContext, state); err != nil {
		result = multierror.Append(result, err)
	}
	return result
}

func (e *Execer) exec(ctx context.Context, state *pipeline.State, spec runtime.Spec, step runtime.Step) error {
	var result error
	select {
	case <-ctx.Done():
		state.Cancel()
		return nil
	default:
	}

	log := logger.FromContext(ctx).WithField("step.name", step.GetName())
	ctx = logger.WithContext(ctx, log)

	if e.sem != nil {
		if err := e.sem.Acquire(ctx, 1); err != nil {
			switch ctx.Err() {
			case context.Canceled, context.DeadlineExceeded:
				state.Cancel()
				return nil
			default:
				return err
			}
		}
		defer e.sem.Release(1)
	}

	switch {
	case state.Cancelled():
		return nil
	case step.GetRunPolicy() == runtime.RunNever:
		return nil
	case step.GetRunPolicy() == runtime.RunAlways:
	case step.GetRunPolicy() == runtime.RunOnFailure && state.Failed() == false:
		state.Skip(step.GetName())
		return e.reporter.ReportStep(noContext, state, step.GetName())
	case step.GetRunPolicy() == runtime.RunOnSuccess && state.Failed():
		state.Skip(step.GetName())
		return e.reporter.ReportStep(noContext, state, step.GetName())
	case state.Finished(step.GetName()):
		return nil
	}

	state.Start(step.GetName())
	if err := e.reporter.ReportStep(noContext, state, step.GetName()); err != nil {
		return err
	}

	copy := step.Clone()
	state.Lock()
	copy.SetEnviron(
		environ.Combine(
			copy.GetEnviron(),
			environ.Build(state.Build),
			environ.Stage(state.Stage),
			environ.Step(findStep(state, step.GetName())),
		),
	)
	state.Unlock()

	engineStep, ok := copy.(*engine.Step)
	if ok {
		if err := e.injectOutputs(spec.(*engine.Spec), engineStep); err != nil {
			state.Fail(step.GetName(), err)
			_ = e.reporter.ReportStep(noContext, state, step.GetName())
			return err
		}
	}

	wc := e.streamer.Stream(noContext, state, step.GetName())
	wc = newReplacer(wc, secretSlice(step))
	ext := extractor.New(wc)

	if step.IsDetached() {
		go func() {
			e.engine.Run(ctx, spec, copy, ext)
			wc.Close()
		}()
		return nil
	}

	exited, err := e.engine.Run(ctx, spec, copy, ext)
	if closeErr := wc.Close(); closeErr != nil {
		result = multierror.Append(result, closeErr)
	}
	if card, ok := ext.File(); ok {
		if uploadErr := e.uploader.UploadCard(ctx, card, state, step.GetName()); uploadErr != nil {
			log.Warnln("cannot upload card")
		}
	}
	switch ctx.Err() {
	case context.Canceled, context.DeadlineExceeded:
		state.Cancel()
		return nil
	}
	if exited != nil {
		if exited.OOMKilled {
			state.Finish(step.GetName(), 137)
		} else {
			state.Finish(step.GetName(), exited.ExitCode)
		}
		if err := e.reporter.ReportStep(noContext, state, step.GetName()); err != nil {
			result = multierror.Append(result, err)
		}
		if exited.ExitCode == 0 {
			if engineStep, ok := copy.(*engine.Step); ok && engineStep.OutputDir != "" && spec.(*engine.Spec).OutputTransport == "file" {
				if err := e.collectOutputs(spec.(*engine.Spec), engineStep); err != nil {
					state.Fail(step.GetName(), err)
					_ = e.reporter.ReportStep(noContext, state, step.GetName())
					return multierror.Append(result, err)
				}
			}
		}
		if exited.ExitCode == 78 {
			state.SkipAll()
		}
		return result
	}
	switch err {
	case context.Canceled, context.DeadlineExceeded:
		state.Cancel()
		return nil
	}
	state.Fail(step.GetName(), err)
	if reportErr := e.reporter.ReportStep(noContext, state, step.GetName()); reportErr != nil {
		result = multierror.Append(result, reportErr)
	}
	return result
}

func (e *Execer) injectOutputs(spec *engine.Spec, step *engine.Step) error {
	if len(step.OutputEnvs) == 0 {
		return nil
	}
	resolved := map[string]string{}
	for envName, ref := range step.OutputEnvs {
		if e.service != nil && spec.PipelineID != "" && spec.OutputTransport != "file" {
			value, ok, err := e.service.Resolve(spec.PipelineID, ref.Step, ref.Key)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("missing outputs from step %q", ref.Step)
			}
			resolved[envName] = value
			continue
		}
		e.mu.RLock()
		values := e.outputs[ref.Step]
		e.mu.RUnlock()
		if len(values) == 0 {
			return fmt.Errorf("missing outputs from step %q", ref.Step)
		}
		value, ok, err := stepoutput.ResolveValue(values, ref.Key)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("missing output %q from step %q", ref.Key, ref.Step)
		}
		resolved[envName] = value
	}
	step.Envs = environ.Combine(step.Envs, resolved)
	return nil
}

func (e *Execer) collectOutputs(spec *engine.Spec, step *engine.Step) error {
	dir := step.OutputDir
	if dir == "" {
		dir = spec.OutputDir
	}
	if dir != "" {
		if _, err := os.Stat(dir); err == nil {
			values, err := stepoutput.Load(dir)
			if err != nil {
				return err
			}
			e.mu.Lock()
			e.outputs[step.Name] = values
			e.mu.Unlock()
			return nil
		}
	}
	if collector, ok := e.engine.(interface {
		CopyOutputs(context.Context, *engine.Step) (map[string]string, error)
	}); ok {
		values, err := collector.CopyOutputs(noContext, step)
		if err != nil {
			return err
		}
		e.mu.Lock()
		e.outputs[step.Name] = values
		e.mu.Unlock()
		return nil
	}
	values, err := stepoutput.Load(dir)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.outputs[step.Name] = values
	e.mu.Unlock()
	return nil
}

func findStep(state *pipeline.State, name string) *drone.Step {
	for _, step := range state.Stage.Steps {
		if step.Name == name {
			return step
		}
	}
	panic("step not found: " + name)
}

func secretSlice(step runtime.Step) []runtime.Secret {
	var secrets []runtime.Secret
	for i := 0; i < step.GetSecretLen(); i++ {
		secrets = append(secrets, step.GetSecretAt(i))
	}
	return secrets
}
