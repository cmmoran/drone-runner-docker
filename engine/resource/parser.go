// Copyright 2019 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package resource

import (
	"errors"
	"fmt"
	"strings"

	"github.com/drone-runners/drone-runner-docker/internal/stepoutput"
	"github.com/drone/runner-go/manifest"

	"github.com/buildkite/yaml"
)

func init() {
	manifest.Register(parse)
}

// parse parses the raw resource and returns an Exec pipeline.
func parse(r *manifest.RawResource) (manifest.Resource, bool, error) {
	if !match(r) {
		return nil, false, nil
	}
	out := new(Pipeline)
	err := yaml.Unmarshal(r.Data, out)
	if err != nil {
		return out, true, err
	}
	err = lint(out)
	return out, true, err
}

// match returns true if the resource matches the kind and type.
func match(r *manifest.RawResource) bool {
	return (r.Kind == Kind && r.Type == Type) ||
		(r.Kind == Kind && r.Type == "")
}

func lint(pipeline *Pipeline) error {
	// ensure pipeline steps are not unique.
	names := map[string]struct{}{}
	for _, step := range pipeline.Steps {
		if step == nil {
			return errors.New("Linter: detected nil step")
		}
		if step.Name == "" {
			return errors.New("Linter: invalid or missing step name")
		}
		if len(step.Name) > 100 {
			return errors.New("Linter: step name cannot exceed 100 characters")
		}
		if _, ok := names[step.Name]; ok {
			return errors.New("Linter: duplicate step name")
		}
		names[step.Name] = struct{}{}
	}

	stepIndex := map[string]*Step{}
	for _, step := range pipeline.Steps {
		stepIndex[step.Name] = step
	}
	for _, step := range pipeline.Steps {
		for envName, variable := range step.Environment {
			if variable == nil || strings.TrimSpace(variable.FromOutput) == "" {
				continue
			}
			ref, err := stepoutput.ParseRef(variable.FromOutput)
			if err != nil {
				return fmt.Errorf("Linter: invalid from_output for %s.%s: %w", step.Name, envName, err)
			}
			producer, ok := stepIndex[ref.Step]
			if !ok {
				return fmt.Errorf("Linter: step %q references unknown output producer %q", step.Name, ref.Step)
			}
			if producer.Detach {
				return fmt.Errorf("Linter: step %q cannot consume outputs from detached step %q", step.Name, ref.Step)
			}
			if !dependsOn(pipeline, step.Name, ref.Step, map[string]bool{}) {
				return fmt.Errorf("Linter: step %q must depend on %q to consume %s", step.Name, ref.Step, envName)
			}
		}
		for settingName, variable := range step.Settings {
			if variable == nil || strings.TrimSpace(variable.FromOutput) == "" {
				continue
			}
			ref, err := stepoutput.ParseRef(variable.FromOutput)
			if err != nil {
				return fmt.Errorf("Linter: invalid from_output for %s.settings.%s: %w", step.Name, settingName, err)
			}
			producer, ok := stepIndex[ref.Step]
			if !ok {
				return fmt.Errorf("Linter: step %q references unknown output producer %q", step.Name, ref.Step)
			}
			if producer.Detach {
				return fmt.Errorf("Linter: step %q cannot consume outputs from detached step %q", step.Name, ref.Step)
			}
			if !dependsOn(pipeline, step.Name, ref.Step, map[string]bool{}) {
				return fmt.Errorf("Linter: step %q must depend on %q to consume setting %s", step.Name, ref.Step, settingName)
			}
		}
	}
	return nil
}

func dependsOn(pipeline *Pipeline, consumer, producer string, seen map[string]bool) bool {
	if consumer == producer {
		return true
	}
	if seen[consumer] {
		return false
	}
	seen[consumer] = true
	current := pipeline.GetStep(consumer)
	if current == nil {
		return false
	}
	for _, dep := range current.DependsOn {
		if dep == producer || dependsOn(pipeline, dep, producer, seen) {
			return true
		}
	}
	return false
}
