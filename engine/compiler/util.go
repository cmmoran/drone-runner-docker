// Copyright 2019 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package compiler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/drone-runners/drone-runner-docker/engine"
	"github.com/drone-runners/drone-runner-docker/engine/resource"
	"github.com/drone-runners/drone-runner-docker/internal/stepoutput"

	"github.com/drone/drone-go/drone"
)

// helper function returns true if the step is configured to
// always run regardless of status.
func isRunAlways(step *resource.Step) bool {
	if len(step.When.Status.Include) == 0 &&
		len(step.When.Status.Exclude) == 0 {
		return false
	}
	return step.When.Status.Match(drone.StatusFailing) &&
		step.When.Status.Match(drone.StatusPassing)
}

// helper function returns true if the step is configured to
// only run on failure.
func isRunOnFailure(step *resource.Step) bool {
	if len(step.When.Status.Include) == 0 &&
		len(step.When.Status.Exclude) == 0 {
		return false
	}
	return step.When.Status.Match(drone.StatusFailing)
}

// helper function returns true if the pipeline specification
// manually defines an execution graph.
func isGraph(spec *engine.Spec) bool {
	for _, step := range spec.Steps {
		if len(step.DependsOn) > 0 {
			return true
		}
	}
	return false
}

// helper function creates the dependency graph for serial
// pipeline execution.
func configureSerial(spec *engine.Spec) {
	var prev *engine.Step
	for _, step := range spec.Steps {
		if prev != nil {
			step.DependsOn = []string{prev.Name}
		}
		prev = step
	}
}

// helper function converts the environment variables to a map,
// returning only inline environment variables not derived from
// a secret.
func convertStaticEnv(src map[string]*resource.Variable) map[string]string {
	dst := map[string]string{}
	for k, v := range src {
		if v == nil {
			continue
		}
		if strings.TrimSpace(v.Secret) == "" && strings.TrimSpace(v.FromOutput) == "" {
			dst[k] = v.Value
		}
	}
	return dst
}

// helper function converts the environment variables to a map,
// returning only inline environment variables not derived from
// a secret.
func convertSecretEnv(src map[string]*resource.Variable) []*engine.Secret {
	dst := []*engine.Secret{}
	for k, v := range src {
		if v == nil {
			continue
		}
		if strings.TrimSpace(v.Secret) != "" {
			dst = append(dst, &engine.Secret{
				Name: v.Secret,
				Mask: true,
				Env:  k,
			})
		}
	}
	return dst
}

func convertOutputEnv(src map[string]*resource.Variable) (map[string]stepoutput.OutputRef, error) {
	dst := map[string]stepoutput.OutputRef{}
	for key, value := range src {
		if value == nil || strings.TrimSpace(value.FromOutput) == "" {
			continue
		}
		ref, err := stepoutput.ParseRef(value.FromOutput)
		if err != nil {
			return nil, err
		}
		dst[key] = ref
	}
	if len(dst) == 0 {
		return nil, nil
	}
	return dst, nil
}

func convertOutputSettings(src map[string]*resource.Parameter) (map[string]stepoutput.OutputRef, error) {
	dst := map[string]stepoutput.OutputRef{}
	for key, value := range src {
		if value == nil || strings.TrimSpace(value.FromOutput) == "" {
			continue
		}
		ref, err := stepoutput.ParseRef(value.FromOutput)
		if err != nil {
			return nil, err
		}
		dst["PLUGIN_"+strings.ToUpper(key)] = ref
	}
	if len(dst) == 0 {
		return nil, nil
	}
	return dst, nil
}

// helper function modifies the pipeline dependency graph to
// account for the clone step.
func configureCloneDeps(spec *engine.Spec) {
	for _, step := range spec.Steps {
		if step.Name == "clone" {
			continue
		}
		if len(step.DependsOn) == 0 {
			step.DependsOn = []string{"clone"}
		}
	}
}

// helper function modifies the pipeline dependency graph to
// account for a disabled clone step.
func removeCloneDeps(spec *engine.Spec) {
	for _, step := range spec.Steps {
		if step.Name == "clone" {
			return
		}
	}
	for _, step := range spec.Steps {
		if len(step.DependsOn) == 1 &&
			step.DependsOn[0] == "clone" {
			step.DependsOn = []string{}
		}
	}
}

func validateOutputRefs(spec *engine.Spec) error {
	steps := map[string]*engine.Step{}
	for _, step := range spec.Steps {
		steps[step.Name] = step
	}
	for _, step := range spec.Steps {
		if step.Detach && len(step.OutputEnvs) != 0 {
			return errors.New("detached steps cannot consume from_output values")
		}
		for envName, ref := range step.OutputEnvs {
			producer, ok := steps[ref.Step]
			if !ok {
				return fmt.Errorf("step %q references unknown output producer %q for %s", step.Name, ref.Step, envName)
			}
			if producer.Detach {
				return fmt.Errorf("step %q cannot consume outputs from detached step %q", step.Name, ref.Step)
			}
			if !dependsOnStep(spec, step.Name, ref.Step, map[string]bool{}) {
				return fmt.Errorf("step %q must depend on %q to consume %s", step.Name, ref.Step, envName)
			}
		}
		if step.Detach && len(step.OutputSettings) != 0 {
			return errors.New("detached steps cannot consume from_output plugin settings")
		}
		for settingName, ref := range step.OutputSettings {
			producer, ok := steps[ref.Step]
			if !ok {
				return fmt.Errorf("step %q references unknown output producer %q for %s", step.Name, ref.Step, settingName)
			}
			if producer.Detach {
				return fmt.Errorf("step %q cannot consume outputs from detached step %q", step.Name, ref.Step)
			}
			if !dependsOnStep(spec, step.Name, ref.Step, map[string]bool{}) {
				return fmt.Errorf("step %q must depend on %q to consume %s", step.Name, ref.Step, settingName)
			}
		}
	}
	return nil
}

func dependsOnStep(spec *engine.Spec, consumer, producer string, seen map[string]bool) bool {
	if consumer == producer {
		return true
	}
	if seen[consumer] {
		return false
	}
	seen[consumer] = true
	for _, step := range spec.Steps {
		if step.Name != consumer {
			continue
		}
		for _, dep := range step.DependsOn {
			if dep == producer || dependsOnStep(spec, dep, producer, seen) {
				return true
			}
		}
	}
	return false
}

// helper function modifies the pipeline dependency graph to
// account for the clone step.
func convertPullPolicy(s string) engine.PullPolicy {
	switch strings.ToLower(s) {
	case "always":
		return engine.PullAlways
	case "if-not-exists":
		return engine.PullIfNotExists
	case "never":
		return engine.PullNever
	default:
		return engine.PullDefault
	}
}

// helper function returns true if the environment variable
// is restricted for internal-use only.
func isRestrictedVariable(env map[string]*resource.Variable) bool {
	for _, name := range restrictedVars {
		if _, ok := env[name]; ok {
			return true
		}
	}
	return false
}

// list of restricted variables
var restrictedVars = []string{
	"XDG_RUNTIME_DIR",
	"DOCKER_OPTS",
	"DOCKER_HOST",
	"PATH",
	"HOME",
}
