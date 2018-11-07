package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"
)

const defaultErrorExitCode = 1

func main() {
	args := os.Args

	// Resolve the command name to an absolute path.
	var err error
	if args[0], err = exec.LookPath(args[0]); err != nil {
		fatal(err)
	}
	if args[0], err = filepath.Abs(args[0]); err != nil {
		fatal(err)
	}

	// The command to be executed by pear.
	cmdName := path.Base(args[0])
	cmdDir := filepath.Dir(args[0])
	args = args[1:]

	config := NewConfig()

	// Set generated environment variables to be available to the
	// configuration except ".sha1" which is set when command logging is
	// performed.
	env := config.GetEnvironment()
	env.Set(".arg0", cmdName)
	env.Set(".cdir", cmdDir)
	env.Set(".home", os.Getenv("HOME"))
	env.Set(".user", os.Getenv("USER"))

	now := time.Now()
	env.Set(".date", fmt.Sprintf("%04d%02d%02d", now.Year(), now.Month(), now.Day()))
	env.Set(".time", fmt.Sprintf("%02d%02d%02d", now.Hour(), now.Minute(), now.Second()))

	wdir, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	env.Set(".wdir", wdir)

	// Parse the configuration.
	// This will read user defined environment variables and commands.
	if err = config.ReadFile(cmdDir + "/../pear.conf"); err != nil {
		fatal(err)
	}

	cmd, err := config.GetCommand(cmdName)
	if err != nil {
		fatal(err)
	}

	err = cmd.Run(env, args)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// The command exited with a non-zero exit code.
			// There is no way to retrieve the exact value in Go (in a portable way).
			os.Exit(defaultErrorExitCode)
		}
		fatal(err)
	}
}
