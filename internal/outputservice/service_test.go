package outputservice

import (
	"testing"
	"time"

	"github.com/drone-runners/drone-runner-docker/internal/outputproto"
)

func TestServiceApplyResolveDelete(t *testing.T) {
	svc := New(time.Hour)
	defer svc.Close()
	svc.RegisterPipeline("1/2/3", map[string]string{"build": "token-build"})

	value := "1.2.3"
	if err := svc.Apply("token-build", []outputproto.OutputOp{{Op: outputproto.OpSet, Key: "version", Value: &value}}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := svc.Resolve("1/2/3", "build", "version")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != "1.2.3" {
		t.Fatalf("want version 1.2.3, got ok=%v value=%q", ok, got)
	}

	if err := svc.Apply("token-build", []outputproto.OutputOp{{Op: outputproto.OpUnset, Key: "version"}}); err != nil {
		t.Fatal(err)
	}
	_, ok, err = svc.Resolve("1/2/3", "build", "version")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected version to be removed")
	}

	svc.DeletePipeline("1/2/3")
	_, ok, err = svc.Resolve("1/2/3", "build", "version")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected deleted pipeline to have no values")
	}
}

func TestServiceTTLExpiry(t *testing.T) {
	svc := New(time.Hour)
	defer svc.Close()
	now := time.Now()
	svc.now = func() time.Time { return now }
	svc.RegisterPipeline("1/2/3", map[string]string{"build": "token-build"})

	now = now.Add(2 * time.Hour)
	svc.prune()
	if err := svc.Apply("token-build", nil); err == nil {
		t.Fatal("expected expired token to fail")
	}
}
