package outputtransport

import (
	"encoding/json"
	"net/http"

	"github.com/drone-runners/drone-runner-docker/internal/outputproto"
	"github.com/drone-runners/drone-runner-docker/internal/outputservice"
)

func NewHTTPHandler(svc *outputservice.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req outputproto.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(outputproto.Response{OK: false, Error: err.Error()})
			return
		}
		if err := req.Validate(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(outputproto.Response{OK: false, Error: err.Error()})
			return
		}
		if err := svc.Apply(req.Token, req.Ops); err != nil {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(outputproto.Response{OK: false, Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(outputproto.Response{OK: true})
	})
}
