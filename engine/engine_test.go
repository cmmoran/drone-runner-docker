package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestCopyOutputs(t *testing.T) {
	client := newMockDockerClient()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "build", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	data := []byte("1.2.3")
	if err := tw.WriteHeader(&tar.Header{Name: "build/version", Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	client.copyReader = io.NopCloser(bytes.NewReader(buf.Bytes()))

	engine := New(client, Opts{})
	got, err := engine.CopyOutputs(context.Background(), &Step{ID: "build-id", OutputDir: "/drone/src/.drone-outputs/build"})
	if err != nil {
		t.Fatal(err)
	}
	if got["version"] != "1.2.3" {
		t.Fatalf("want version 1.2.3, got %q", got["version"])
	}
}

func TestCopyOutputs_MissingPathReturnsEmpty(t *testing.T) {
	client := newMockDockerClient()
	client.copyErr = cerrdefs.ErrNotFound

	engine := New(client, Opts{})
	got, err := engine.CopyOutputs(context.Background(), &Step{ID: "build-id", OutputDir: "/drone/src/.drone-outputs/clone"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want no outputs, got %#v", got)
	}
}

func TestDestroy_StopsContainersGracefully(t *testing.T) {
	client := newMockDockerClient()
	engine := New(client, Opts{})
	spec := &Spec{
		Steps: []*Step{
			{ID: "step_1"},
		},
	}

	if err := engine.Destroy(context.Background(), spec); err != nil {
		t.Fatalf("Destroy returned error: %v", err)
	}

	if len(client.stopCalls) != 1 {
		t.Fatalf("expected 1 stop call, got %d", len(client.stopCalls))
	}
	if client.stopCalls[0].id != "step_1" {
		t.Fatalf("expected stop for step_1, got %s", client.stopCalls[0].id)
	}
	if client.stopCalls[0].options.Timeout != nil {
		t.Fatalf("expected nil stop timeout to use container StopTimeout, got %v", *client.stopCalls[0].options.Timeout)
	}
	if len(client.killCalls) != 0 {
		t.Fatalf("expected no kill calls, got %d", len(client.killCalls))
	}
}

func TestDestroy_KillsContainerWhenGracefulStopFails(t *testing.T) {
	client := newMockDockerClient()
	client.stopErr = errors.New("stop failed")
	engine := New(client, Opts{})
	spec := &Spec{
		Steps: []*Step{
			{ID: "step_1"},
		},
	}

	if err := engine.Destroy(context.Background(), spec); err != nil {
		t.Fatalf("Destroy returned error: %v", err)
	}

	if len(client.stopCalls) != 1 {
		t.Fatalf("expected 1 stop call, got %d", len(client.stopCalls))
	}
	if len(client.killCalls) != 1 {
		t.Fatalf("expected 1 kill call, got %d", len(client.killCalls))
	}
	if client.killCalls[0].id != "step_1" {
		t.Fatalf("expected kill for step_1, got %s", client.killCalls[0].id)
	}
	if client.killCalls[0].signal != "9" {
		t.Fatalf("expected kill signal 9, got %s", client.killCalls[0].signal)
	}
}

type mockDockerClient struct {
	stopErr    error
	stopCalls  []stopCall
	killCalls  []killCall
	copyReader io.ReadCloser
	copyErr    error
}

type stopCall struct {
	id      string
	options container.StopOptions
}

type killCall struct {
	id     string
	signal string
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{}
}

func (*mockDockerClient) Ping(context.Context) (types.Ping, error) {
	return types.Ping{}, nil
}

func (*mockDockerClient) VolumeCreate(context.Context, volume.CreateOptions) (volume.Volume, error) {
	return volume.Volume{}, nil
}

func (*mockDockerClient) NetworkCreate(context.Context, string, network.CreateOptions) (network.CreateResponse, error) {
	return network.CreateResponse{}, nil
}

func (m *mockDockerClient) ContainerStop(_ context.Context, containerID string, options container.StopOptions) error {
	m.stopCalls = append(m.stopCalls, stopCall{id: containerID, options: options})
	return m.stopErr
}

func (m *mockDockerClient) ContainerKill(_ context.Context, containerID, signal string) error {
	m.killCalls = append(m.killCalls, killCall{id: containerID, signal: signal})
	return nil
}

func (*mockDockerClient) ContainerRemove(context.Context, string, container.RemoveOptions) error {
	return nil
}

func (*mockDockerClient) VolumeRemove(context.Context, string, bool) error {
	return nil
}

func (*mockDockerClient) NetworkRemove(context.Context, string) error {
	return nil
}

func (*mockDockerClient) ImagePull(context.Context, string, image.PullOptions) (io.ReadCloser, error) {
	return nil, nil
}

func (*mockDockerClient) ContainerCreate(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *ocispec.Platform, string) (container.CreateResponse, error) {
	return container.CreateResponse{}, nil
}

func (*mockDockerClient) NetworkConnect(context.Context, string, string, *network.EndpointSettings) error {
	return nil
}

func (*mockDockerClient) ContainerStart(context.Context, string, container.StartOptions) error {
	return nil
}

func (*mockDockerClient) ContainerWait(context.Context, string, container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	waitCh := make(chan container.WaitResponse)
	errCh := make(chan error)
	return waitCh, errCh
}

func (*mockDockerClient) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

func (*mockDockerClient) ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockDockerClient) CopyFromContainer(context.Context, string, string) (io.ReadCloser, container.PathStat, error) {
	if m.copyErr != nil {
		return nil, container.PathStat{}, m.copyErr
	}
	if m.copyReader != nil {
		return m.copyReader, container.PathStat{}, nil
	}
	return io.NopCloser(strings.NewReader("")), container.PathStat{}, nil
}
