package outputtransport

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/drone-runners/drone-runner-docker/internal/outputproto"
	"github.com/drone-runners/drone-runner-docker/internal/outputservice"
)

func TestListenUnixAppliesOutputs(t *testing.T) {
	svc := outputservice.New(time.Hour)
	defer svc.Close()
	svc.RegisterPipeline("1/2/3", map[string]string{"build": "token-build"})

	socketPath := filepath.Join(t.TempDir(), "outputs.sock")
	closer, err := ListenUnix(socketPath, svc)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req := outputproto.Request{
		Version: outputproto.Version,
		Token:   "token-build",
		Ops:     []outputproto.OutputOp{{Op: outputproto.OpSet, Key: "version", Value: stringPtr("1.2.3")}},
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatal(err)
	}
	var resp outputproto.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("want success response, got %+v", resp)
	}
	got, ok, err := svc.Resolve("1/2/3", "build", "version")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != "1.2.3" {
		t.Fatalf("want stored version, got ok=%v value=%q", ok, got)
	}
}

func stringPtr(s string) *string { return &s }
