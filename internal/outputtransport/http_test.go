package outputtransport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/drone-runners/drone-runner-docker/internal/outputproto"
	"github.com/drone-runners/drone-runner-docker/internal/outputservice"
)

func TestHTTPHandlerAppliesOutputs(t *testing.T) {
	svc := outputservice.New(time.Hour)
	defer svc.Close()
	svc.RegisterPipeline("1/2/3", map[string]string{"build": "token-build"})

	body, err := json.Marshal(outputproto.Request{
		Version: outputproto.Version,
		Token:   "token-build",
		Ops:     []outputproto.OutputOp{{Op: outputproto.OpSet, Key: "version", Value: stringPtr("1.2.3")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/outputs", bytes.NewReader(body))
	NewHTTPHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	got, ok, err := svc.Resolve("1/2/3", "build", "version")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != "1.2.3" {
		t.Fatalf("want stored version, got ok=%v value=%q", ok, got)
	}
}
