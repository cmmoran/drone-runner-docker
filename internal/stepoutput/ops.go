package stepoutput

import "github.com/drone-runners/drone-runner-docker/internal/outputproto"

func PatchToOps(patch Patch) []outputproto.OutputOp {
	ops := make([]outputproto.OutputOp, 0, len(patch))
	for key, value := range patch {
		if value == nil {
			ops = append(ops, outputproto.OutputOp{
				Op:  outputproto.OpUnset,
				Key: key,
			})
			continue
		}
		copied := *value
		ops = append(ops, outputproto.OutputOp{
			Op:    outputproto.OpSet,
			Key:   key,
			Value: &copied,
		})
	}
	return ops
}
