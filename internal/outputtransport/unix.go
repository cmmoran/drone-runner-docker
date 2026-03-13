package outputtransport

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/drone-runners/drone-runner-docker/internal/outputproto"
	"github.com/drone-runners/drone-runner-docker/internal/outputservice"
)

func ListenUnix(socketPath string, svc *outputservice.Service) (io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	go serveUnix(ln, svc)
	return closerFunc(func() error {
		err := ln.Close()
		_ = os.Remove(socketPath)
		return err
	}), nil
}

func serveUnix(ln net.Listener, svc *outputservice.Service) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleUnixConn(conn, svc)
	}
}

func handleUnixConn(conn net.Conn, svc *outputservice.Service) {
	defer conn.Close()
	var req outputproto.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(outputproto.Response{OK: false, Error: err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		_ = json.NewEncoder(conn).Encode(outputproto.Response{OK: false, Error: err.Error()})
		return
	}
	if err := svc.Apply(req.Token, req.Ops); err != nil {
		_ = json.NewEncoder(conn).Encode(outputproto.Response{OK: false, Error: err.Error()})
		return
	}
	_ = json.NewEncoder(conn).Encode(outputproto.Response{OK: true})
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
