package outputproto

import (
	"errors"
	"fmt"
	"strings"
)

const Version = 1

type OpType string

const (
	OpSet   OpType = "set"
	OpUnset OpType = "unset"
)

type OutputOp struct {
	Op    OpType  `json:"op"`
	Key   string  `json:"key"`
	Value *string `json:"value,omitempty"`
}

type Request struct {
	Version int        `json:"version"`
	Token   string     `json:"token"`
	Ops     []OutputOp `json:"ops"`
}

type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (r Request) Validate() error {
	if r.Version != Version {
		return fmt.Errorf("unsupported output protocol version: %d", r.Version)
	}
	if strings.TrimSpace(r.Token) == "" {
		return errors.New("missing output token")
	}
	if len(r.Ops) == 0 {
		return errors.New("missing output ops")
	}
	for _, op := range r.Ops {
		if err := op.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (o OutputOp) Validate() error {
	switch o.Op {
	case OpSet:
		if o.Value == nil {
			return fmt.Errorf("missing value for output op %q", o.Key)
		}
	case OpUnset:
		if o.Value != nil {
			return fmt.Errorf("unexpected value for unset op %q", o.Key)
		}
	default:
		return fmt.Errorf("unsupported output op: %q", o.Op)
	}
	if strings.TrimSpace(o.Key) == "" {
		return errors.New("missing output key")
	}
	return nil
}
