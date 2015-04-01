// The command package provides the implementation the command protocol for the sphere-client process.
// The implementation delegates execution to a helper script. The helper script is responsible for
// validating that the command is a supported command and for generating a json output.
package command

import (
	"encoding/json"
	"fmt"
	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/logger"
	"os"
)

type Configuration struct {
	Conn       *ninja.Connection
	Device     ninja.Device
	HelperPath string
}

// Format of a request to execute a command
type Request struct {
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Async   bool     `json:"async,omitempty"`
}

// Format of a response from a command.
type Response struct {
	Data   interface{} `json:"data,omitempty"`   // present if the command generates json structured output
	Output *[]string   `json:"output,omitempty"` // present if the command does not generate json structured output
	Error  *[]string   `json:"error,omitempty"`  // any output written to the commands stderr
	Status *int        `json:"status,omitempty"` // the exit status of the command.
	Pid    *int        `json:"pid,omitempty"`    // the PID of the started or completed command.
}

// The command service.
type CommandChannel struct {
	config Configuration
	sender func(event string, payload ...interface{}) error
}

var log = logger.GetLogger("sphere-client.command")

// Exports the command service into the bus
func ExportCommandChannel(config *Configuration) error {
	if config == nil {
		return fmt.Errorf("illegal argument: config == nil")
	}

	channel := &CommandChannel{
		config: *config,
	}
	return config.Conn.ExportChannel(config.Device, channel, "command")
}

func (cs *CommandChannel) GetProtocol() string {
	return "http://schema.ninjablocks.com/protocol/command"
}

func (cs *CommandChannel) SetEventHandler(sender func(event string, payload ...interface{}) error) {
	cs.sender = sender
}

// Support for the exec method.
func (cs *CommandChannel) Exec(request *Request) (*Response, error) {
	if fromChild, toParent, err := os.Pipe(); err != nil {
		return nil, err
	} else if fromParent, toChild, err := os.Pipe(); err != nil {
		return nil, err
	} else {
		procAttr := &os.ProcAttr{
			Files: []*os.File{
				fromParent,
				toParent,
				os.Stderr,
			},
		}
		args := make([]string, len(request.Args)+3)
		args[0] = cs.config.HelperPath
		args[1] = "exec"
		args[2] = request.Command
		copy(args[3:], request.Args)
		if proc, err := os.StartProcess(cs.config.HelperPath, args, procAttr); err != nil {
			return nil, err
		} else {

			fromParent.Close()
			toParent.Close()

			toChild.Close()

			response := &Response{
				Pid: &proc.Pid,
			}

			executor := func(response *Response) error {

				err := json.NewDecoder(fromChild).Decode(response)
				if err != nil {
					log.Errorf("failed to parse response from command: %v", err)
				}

				cs.sender("completed", response)
				fromChild.Close()
				return err
			}

			if request.Async {
				asyncResponse := &Response{
					Pid: &proc.Pid,
				}
				go executor(asyncResponse)
				return response, nil
			}

			return response, executor(response)
		}
	}

}
