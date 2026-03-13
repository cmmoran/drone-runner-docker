// Copyright 2019 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package command

import (
	"context"
	"os"

	"github.com/drone-runners/drone-runner-docker/command/daemon"
	"github.com/drone-runners/drone-runner-docker/version"

	"gopkg.in/alecthomas/kingpin.v2"
)

// empty context
var nocontext = context.Background()

// Command parses the command line arguments and then executes a
// subcommand program.
func Command() {
	app := kingpin.New("drone", "drone docker runner")
	registerCompile(app)
	registerExec(app)
	registerCopy(app)
	registerOutput(app)
	daemon.Register(app)

	kingpin.Version(version.Version)
	kingpin.MustParse(app.Parse(normalizeOutputInvocation(os.Args)))
}
