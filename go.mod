module github.com/drone-runners/drone-runner-docker

go 1.25.7

//replace github.com/docker/docker => github.com/docker/engine v17.12.0-ce-rc1.0.20200309214505-aa6a9891b09c+incompatible

require (
	github.com/buildkite/yaml v2.1.0+incompatible
	github.com/cmmoran/envconfig v0.0.0-20250930083126-8ea393d1336b
	github.com/dchest/uniuri v1.2.0
	github.com/distribution/reference v0.6.0
	github.com/docker/docker v28.5.2+incompatible
	github.com/drone/drone-go v1.7.1
	github.com/drone/envsubst v1.0.3
	github.com/drone/runner-go v1.13.0
	github.com/drone/signal v1.0.0
	github.com/ghodss/yaml v1.0.0
	github.com/google/go-cmp v0.7.0
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-isatty v0.0.20
	github.com/sirupsen/logrus v1.9.4
	golang.org/x/sync v0.19.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

require (
	github.com/99designs/basicauth-go v0.0.0-20230316000542-bf6f9cbbf0f8 // indirect
	github.com/99designs/httpsignatures-go v0.0.0-20170731043157-88528bf4ca7e // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/alecthomas/template v0.0.0-20160405071501-a0175ee3bccc // indirect
	github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/natessilva/dag v0.0.0-20180124060714-7194b8dcc5c4 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.46.1 // indirect
	go.opentelemetry.io/otel v1.40.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/sdk v1.21.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	golang.org/x/time v0.0.0-20181108054448-85acf8d2951c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)
