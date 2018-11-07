package main

import (
	"fmt"
	"os"
)

// Write error to stderr and exit with defaultErrorExitCode
func fatal(a ...interface{}) {
	fmt.Fprint(os.Stderr, append(a, "\n")...)
	os.Exit(defaultErrorExitCode)
}

// Write error to stderr and exit with defaultErrorExitCode
func fatalf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(defaultErrorExitCode)
}
