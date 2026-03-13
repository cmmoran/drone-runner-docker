package outputexec

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/drone-runners/drone-runner-docker/engine"
	"github.com/drone-runners/drone-runner-docker/internal/stepoutput"

	"github.com/drone/drone-go/drone"
	"github.com/drone/runner-go/pipeline"
	runtime "github.com/drone/runner-go/pipeline/runtime"
)

func TestExec_PropagatesStepOutputs(t *testing.T) {
	outputDir, err := os.MkdirTemp("", "outputexec-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outputDir)

	spec := &engine.Spec{
		OutputDir:       outputDir,
		OutputTransport: "file",
		Steps: []*engine.Step{
			{
				Name:      "build",
				Envs:      map[string]string{"DRONE_OUTPUT_DIR": filepath.Join(outputDir, "build")},
				OutputDir: filepath.Join(outputDir, "build"),
			},
			{
				Name:      "publish",
				DependsOn: []string{"build"},
				Envs:      map[string]string{},
				OutputEnvs: map[string]stepoutput.OutputRef{
					"VERSION": {Step: "build", Key: "version"},
				},
			},
		},
	}

	state := &pipeline.State{
		Build: &drone.Build{},
		Repo:  &drone.Repo{},
		Stage: &drone.Stage{
			Status: drone.StatusPending,
			Steps: []*drone.Step{
				{Name: "build", Status: drone.StatusPending},
				{Name: "publish", Status: drone.StatusPending},
			},
		},
		System: &drone.System{},
	}

	fake := &fakeEngine{}
	exec := New(pipeline.NopReporter(), pipeline.NopStreamer(), pipeline.NopUploader(), fake, 0, nil)
	if err := exec.Exec(context.Background(), spec, state); err != nil {
		t.Fatal(err)
	}
	if got := fake.publishVersion; got != "1.2.3" {
		t.Fatalf("want VERSION=1.2.3, got %q", got)
	}
}

type fakeEngine struct {
	publishVersion string
}

func (*fakeEngine) Setup(context.Context, runtime.Spec) error   { return nil }
func (*fakeEngine) Destroy(context.Context, runtime.Spec) error { return nil }

func (f *fakeEngine) Run(_ context.Context, spec runtime.Spec, step runtime.Step, _ io.Writer) (*runtime.State, error) {
	engineStep := step.(*engine.Step)
	if engineStep.Name == "build" {
		dir := engineStep.OutputDir
		if dir == "" {
			dir = filepath.Join(spec.(*engine.Spec).OutputDir, "build")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, "version"), []byte("1.2.3"), 0o644); err != nil {
			return nil, err
		}
	}
	if engineStep.Name == "publish" {
		f.publishVersion = engineStep.Envs["VERSION"]
	}
	return &runtime.State{Exited: true, ExitCode: 0}, nil
}
