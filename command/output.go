package command

import (
	"fmt"
	"os"

	"github.com/drone-runners/drone-runner-docker/internal/stepoutput"

	"gopkg.in/alecthomas/kingpin.v2"
)

type outputCommand struct{}

type outputSetCommand struct {
	format string
	key    string
	value  string
}

type outputPutCommand struct {
	format string
	path   string
}

type outputUnsetCommand struct {
	key string
}

func (c *outputSetCommand) run(*kingpin.ParseContext) error {
	dir, err := outputDir()
	if err != nil {
		return err
	}
	value, _, err := stepoutput.ParseSetValue(c.value, c.format)
	if err != nil {
		return err
	}
	patch, err := stepoutput.FlattenUnder(c.key, value)
	if err != nil {
		return err
	}
	return stepoutput.ApplyPatch(dir, patch)
}

func (c *outputPutCommand) run(*kingpin.ParseContext) error {
	dir, err := outputDir()
	if err != nil {
		return err
	}
	value, err := stepoutput.ParsePutFile(c.path, c.format)
	if err != nil {
		return err
	}
	patch, err := stepoutput.FlattenRoot(value)
	if err != nil {
		return err
	}
	return stepoutput.ApplyPatch(dir, patch)
}

func (c *outputUnsetCommand) run(*kingpin.ParseContext) error {
	dir, err := outputDir()
	if err != nil {
		return err
	}
	return stepoutput.Unset(dir, c.key)
}

func registerOutput(app *kingpin.Application) {
	root := app.Command("output", "manages step outputs")

	setCmd := new(outputSetCommand)
	set := root.Command("set", "sets a step output").Action(setCmd.run)
	set.Flag("format", "optional input format").EnumVar(&setCmd.format, "env", "json", "yaml", "toml")
	set.Arg("key", "output key").Required().StringVar(&setCmd.key)
	set.Arg("value", "output value").Required().StringVar(&setCmd.value)

	putCmd := new(outputPutCommand)
	put := root.Command("put", "imports step outputs from a file").Action(putCmd.run)
	put.Flag("format", "optional input format").EnumVar(&putCmd.format, "env", "json", "yaml", "toml")
	put.Arg("path", "input file").Required().StringVar(&putCmd.path)

	unsetCmd := new(outputUnsetCommand)
	unset := root.Command("unset", "removes a step output").Action(unsetCmd.run)
	unset.Arg("key", "output key").Required().StringVar(&unsetCmd.key)
}

func normalizeOutputInvocation(args []string) []string {
	if len(args) == 0 {
		return args
	}
	switch filepathBase(args[0]) {
	case "drone-output":
		return append([]string{"output"}, args[1:]...)
	default:
		return args[1:]
	}
}

func filepathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func outputDir() (string, error) {
	dir := os.Getenv("DRONE_OUTPUT_DIR")
	if dir == "" {
		return "", fmt.Errorf("DRONE_OUTPUT_DIR is not set")
	}
	return dir, nil
}
