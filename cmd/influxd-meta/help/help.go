package help

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Command displays help for command-line sub-commands.
type Command struct {
	Stdout io.Writer
}

// NewCommand returns a new instance of Command.
func NewCommand() *Command {
	return &Command{
		Stdout: os.Stdout,
	}
}

// Run executes the command.
func (cmd *Command) Run(args ...string) error {
	fmt.Fprintln(cmd.Stdout, strings.TrimSpace(usage))
	return nil
}

const usage = `
usage: influxd-meta [flags]

InfluxDB Meta is a raft-based meta service for InfluxDB. With this, influxDB can be distributed in a cluster.

The commands are:

    backup               downloads a snapshot of a data node and saves it to disk
    config               display the default configuration
    help                 display this help message
    run                  run node with existing configuration
    version              displays the InfluxDB version


Options:
	-config <path>       Set the path to the configuration file.

	-single-server       Start the server in single server mode.  This
	                     will trigger an election.  If you are starting
	                     multiple nodes to form a cluster, this option
	                     should not be used.

	-hostname <name>     Override the hostname, the 'hostname' configuration
	                     option will be overridden.

	-pidfile <path>      Write process ID to a file.

	-cpuprofile <path>   Write CPU profiling information to a file.

	-memprofile <path>   Write memory usage information to a file.


Use "influxd-meta [command] -help" for more information about a command.
`
