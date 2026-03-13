# drone-runner-docker

The `docker` runner executes pipelines inside Docker containers. This runner is intended for linux workloads that are suitable for execution inside containers. This requires Drone server `1.6.0` or higher.

## Step Outputs

This fork adds runner-side step outputs.

A producing step can publish values with `drone-output`, and a downstream step
can consume those values with `from_output` in `environment`.

Example producer:

```yaml
steps:
  - name: build
    commands:
      - drone-output set version 1.2.3
      - drone-output set --format json image '{"name":"repo/app","digest":"sha256:abc"}'
      - drone-output put metadata.yaml
      - drone-output unset image.digest
```

Example consumer:

```yaml
steps:
  - name: publish
    depends_on: [build]
    environment:
      VERSION:
        from_output: build.version
      IMAGE_NAME:
        from_output: build.image.name
      LAST_TAG:
        from_output: build.tags.-1
```

Notes:

- Linux-only
- runner-only
- same pipeline execution and same stage only
- outputs are usable in `environment`
- no Drone server changes are required

Supported `from_output` forms:

- `build.version`
- `build/version`
- `steps.build.outputs.version`
- `step=build,key=version`

Canonical form:

- `build.version`

Negative list indexes are supported only when reading, and only as the final
path segment:

- `build.tags.-1`
- `build.tags.-2`

### Helper Injection

The runner injects a helper binary named `drone-output` into Linux step
containers. The CLI is:

- `drone-output set [--format env|json|yaml|toml] <key> <value>`
- `drone-output put [--format env|json|yaml|toml] <path>`
- `drone-output unset <key>`

Structured values are flattened into dot-path keys:

- maps become `image.name`
- lists become `tags.0`, `tags.1`

### Output Transport

The current transport modes are:

- `file`
- `auto`
- `unix`
- `http`

Default runner behavior in this fork is `auto`.

Selection order:

1. `unix` if a shared IPC mount is available to both the runner and step containers
2. `http` if bind and advertise settings are configured
3. `file` as fallback

Typical Unix IPC configuration:

```text
DRONE_OUTPUT_TRANSPORT=auto
DRONE_OUTPUT_SOCKET_ROOT=/drone/outputs
DRONE_RUNNER_VOLUMES=/srv/data/platform/tools/drone-runner/drone-outputs:/drone/outputs
```

For HTTP IPC, both of these must be configured:

```text
DRONE_OUTPUT_HTTP_BIND=:3001
DRONE_OUTPUT_HTTP_ADVERTISE=http://<runner-reachable-name>:3001/outputs
```

### More Detail

Full feature documentation:

- [docs/step-outputs.md](docs/step-outputs.md)

Transport and architecture notes:

- [docs/output-transport-architecture.md](docs/output-transport-architecture.md)

Documentation:<br/>
https://docs.drone.io/runner/docker/overview/

Technical Support:<br/>
https://discourse.drone.io

Issue Tracker and Roadmap:<br/>
https://trello.com/b/ttae5E5o/drone

## Release procedure

Run the changelog generator.

```BASH
docker run -it --rm -v "$(pwd)":/usr/local/src/your-app githubchangeloggenerator/github-changelog-generator -u drone-runners -p drone-runner-docker -t <secret github token>
```

You can generate a token by logging into your GitHub account and going to Settings -> Personal access tokens.

Next we tag the PR's with the fixes or enhancements labels. If the PR does not fufil the requirements, do not add a label.

**Before moving on make sure to update the version file `version/version.go && version/version_test.go`.**

Run the changelog generator again with the future version according to semver.

```BASH
docker run -it --rm -v "$(pwd)":/usr/local/src/your-app githubchangeloggenerator/github-changelog-generator -u drone-runners -p drone-runner-docker -t <secret token> --future-release v1.0.0
```

Create your pull request for the release. Get it merged then tag the release.
