package command

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/drone-runners/drone-runner-docker/internal/outputproto"
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
	value, _, err := stepoutput.ParseSetValue(c.value, c.format)
	if err != nil {
		return err
	}
	patch, err := stepoutput.FlattenUnder(c.key, value)
	if err != nil {
		return err
	}
	return sendPatch(patch)
}

func (c *outputPutCommand) run(*kingpin.ParseContext) error {
	value, err := stepoutput.ParsePutFile(c.path, c.format)
	if err != nil {
		return err
	}
	patch, err := stepoutput.FlattenRoot(value)
	if err != nil {
		return err
	}
	return sendPatch(patch)
}

func (c *outputUnsetCommand) run(*kingpin.ParseContext) error {
	return sendPatch(stepoutput.Patch{c.key: nil})
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

func sendPatch(patch stepoutput.Patch) error {
	switch strings.ToLower(os.Getenv("DRONE_OUTPUT_TRANSPORT")) {
	case "", "file":
		dir, err := outputDir()
		if err != nil {
			return err
		}
		return stepoutput.ApplyPatch(dir, patch)
	case "unix":
		return sendRequest(newRequest(patch), unixSender{})
	case "http":
		return sendRequest(newRequest(patch), httpSender{})
	default:
		return fmt.Errorf("unsupported output transport: %s", os.Getenv("DRONE_OUTPUT_TRANSPORT"))
	}
}

func newRequest(patch stepoutput.Patch) outputproto.Request {
	return outputproto.Request{
		Version: outputproto.Version,
		Token:   os.Getenv("DRONE_OUTPUT_TOKEN"),
		Ops:     stepoutput.PatchToOps(patch),
	}
}

type outputSender interface {
	Send(outputproto.Request) error
}

type unixSender struct{}

func (unixSender) Send(req outputproto.Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	socketPath := os.Getenv("DRONE_OUTPUT_SOCKET")
	if socketPath == "" {
		return fmt.Errorf("DRONE_OUTPUT_SOCKET is not set")
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	var resp outputproto.Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return err
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "output request failed"
		}
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

type httpSender struct{}

func (httpSender) Send(req outputproto.Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	url := os.Getenv("DRONE_OUTPUT_URL")
	if url == "" {
		return fmt.Errorf("DRONE_OUTPUT_URL is not set")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var outputResp outputproto.Response
	if err := json.NewDecoder(resp.Body).Decode(&outputResp); err != nil {
		return err
	}
	if !outputResp.OK {
		if outputResp.Error == "" {
			outputResp.Error = "output request failed"
		}
		return fmt.Errorf("%s", outputResp.Error)
	}
	return nil
}

func sendRequest(req outputproto.Request, sender outputSender) error {
	return sender.Send(req)
}
