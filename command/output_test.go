package command

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/drone-runners/drone-runner-docker/internal/outputservice"
	"github.com/drone-runners/drone-runner-docker/internal/outputtransport"
	"github.com/drone-runners/drone-runner-docker/internal/stepoutput"
)

func TestSendPatchFile(t *testing.T) {
	dir := t.TempDir()
	version := "1.2.3"
	t.Setenv("DRONE_OUTPUT_TRANSPORT", "file")
	t.Setenv("DRONE_OUTPUT_DIR", dir)

	if err := sendPatch(stepoutput.Patch{"version": &version}); err != nil {
		t.Fatal(err)
	}

	got, err := readFile(filepath.Join(dir, "version"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.3" {
		t.Fatalf("want version 1.2.3, got %q", got)
	}
}

func TestSendPatchUnix(t *testing.T) {
	svc := outputservice.New(0)
	defer svc.Close()
	svc.RegisterPipeline("1/2/3", map[string]string{"build": "token-build"})
	version := "1.2.3"

	socketPath := filepath.Join(t.TempDir(), "outputs.sock")
	closer, err := outputtransport.ListenUnix(socketPath, svc)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	t.Setenv("DRONE_OUTPUT_TRANSPORT", "unix")
	t.Setenv("DRONE_OUTPUT_SOCKET", socketPath)
	t.Setenv("DRONE_OUTPUT_TOKEN", "token-build")

	if err := sendPatch(stepoutput.Patch{"version": &version}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := svc.Resolve("1/2/3", "build", "version")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != "1.2.3" {
		t.Fatalf("want version 1.2.3, got %q ok=%v", got, ok)
	}
}

func TestSendPatchHTTP(t *testing.T) {
	svc := outputservice.New(0)
	defer svc.Close()
	svc.RegisterPipeline("1/2/3", map[string]string{"build": "token-build"})
	version := "1.2.3"

	server := httptest.NewServer(outputtransport.NewHTTPHandler(svc))
	defer server.Close()

	t.Setenv("DRONE_OUTPUT_TRANSPORT", "http")
	t.Setenv("DRONE_OUTPUT_URL", server.URL)
	t.Setenv("DRONE_OUTPUT_TOKEN", "token-build")

	if err := sendPatch(stepoutput.Patch{"version": &version}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := svc.Resolve("1/2/3", "build", "version")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != "1.2.3" {
		t.Fatalf("want version 1.2.3, got %q ok=%v", got, ok)
	}
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
