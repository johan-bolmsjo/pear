package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func (cmd *Command) Run(env Environment, args []string) error {
	execName := env.Expand(cmd.exec)
	if execName == "" {
		return fmt.Errorf("executable name missing for command '%s'", cmd.name)
	}

	for _, s := range cmd.append {
		args = append(args, env.Expand(s))
	}
	t := make([]string, len(args)+len(cmd.prepend))
	copy(t[len(cmd.prepend):], args)
	args = t
	for i, s := range cmd.prepend {
		args[i] = env.Expand(s)
	}

	args = filterOut(args, cmd.filterOut)

	// Do command logging if enabled.
	if cmd.logfile != "" {
		logentry := buildLogentry(execName, args)
		updateLogfileEnv(env, args, logentry)
		if err := writeLogfile(env.Expand(cmd.logfile), logentry); err != nil {
			return err
		}
	}

	// Do scrubbed logging for rtags indexer if enabled.
	if cmd.rtags_logfile != "" {
		logentry := buildRtagsLogentry(env.Get(".wdir"), execName, args)
		if len(logentry) > 0 {
			updateLogfileEnv(env, args, logentry)
			if err := writeLogfile(env.Expand(cmd.rtags_logfile), logentry); err != nil {
				return err
			}
		}
	}

	osCmd := exec.Command(execName, args...)

	// Add the path of the wrapper symlink first to the PATH environment
	// variable so that the wrapper for the linker is executed when the
	// compiler does the linking.
	osCmd.Env = prependEnvPath(os.Environ(), env.Get(".cdir"))

	osCmd.Stdin = os.Stdin
	osCmd.Stdout = os.Stdout
	osCmd.Stderr = os.Stderr
	return osCmd.Run()
}

func makeInputOutputPath(basedir, path, fallback string) string {
	if path == "" {
		return fallback
	} else {
		return makeAbsolutePath(basedir, path)
	}
}

// Update environment for expanding log file file names.
func updateLogfileEnv(env Environment, args []string, logentry []byte) {
	sha1Sum := sha1.Sum(logentry)
	sha1SumHex := hex.EncodeToString(sha1Sum[:])
	env.Set(".sha1", sha1SumHex)
	input, output := getCmdlineInputOutput(args)
	wdir := env.Get(".wdir")
	env.Set(".input", makeInputOutputPath(wdir, input, sha1SumHex))
	env.Set(".output", makeInputOutputPath(wdir, output, sha1SumHex))
}

// Get output file from command line.
// If no output is found an empty string is returned.
func getCmdlineInputOutput(args []string) (input, output string) {
	var sources []string
	var hasCompileFlag bool

	re := regexp.MustCompile(`^(-o(.+)|([^-].*\.(c|i|ii|cc|cp|cxx|cpp|c\+\+|C|f|F|r|s|S)))$`)

	// Regexp groups
	const (
		outputGroup = 2
		sourceGroup = 3
	)

	const (
		stateInitial int = iota
		stateOutputArg
	)
	state := stateInitial

next_arg:
	for _, arg := range args {
		if state == stateOutputArg {
			output = arg
			state = stateInitial
			continue
		}

		switch arg {
		case "-o":
			state = stateOutputArg
			continue next_arg
		case "-c":
			hasCompileFlag = true
			continue next_arg
		}

		match := re.FindStringSubmatch(arg)
		if len(match) == 0 {
			continue
		}

		if t := match[outputGroup]; t != "" {
			output = t
		}
		if t := match[sourceGroup]; t != "" {
			sources = append(sources, t)
		}
	}

	if len(sources) == 1 {
		input = sources[0]
	}
	if output == "" && hasCompileFlag && input != "" {
		output = input + ".o"
	}
	return
}

func buildLogentry(execName string, args []string) []byte {
	return []byte(execName + " " + strings.Join(args, " ") + "\n")
}

// Arguments that should be completely filtered out
var rtagsFilterRegexp = regexp.MustCompile(`^-(fvar-tracking-assignments|fdebug-prefix-map|falign-functions)`)

// Arguments that should be scrubbed in some way
var rtagsScrubRegexp *regexp.Regexp

func init() {
	singleArgExpr := `(-(-sysroot|isystem|o|I|M|MD|MG|MM|MMD|MP|MF|MQ|MT))`
	compundArgExpr := `(-(-sysroot=|o|I|MF|MQ|MT))(.+)`
	fileArgExpr := `([^-].*\.(c|i|ii|cc|cp|cxx|cpp|c\+\+|C|f|F|r|s|S))`
	rspFileArgExpr := `(@(.*\.rsp))`

	rtagsScrubRegexp = regexp.MustCompile("^(" + singleArgExpr + "|" + compundArgExpr + "|" + fileArgExpr + "|" + rspFileArgExpr + ")$")
}

// Build scrubbed log file output for rtags indexer.
// Relative paths are converted to absolute and some arguments are removed that
// the rtags indexer barfs on. Additionally some scrubbing is done to avoid
// generating duplicate logs when invoked by ccache.
//
// CMake issues:
//
// * CMake may pass compiler directives using rsp files.
//   The rsp files are removed after compilation making indexing impossible.
//
// rtags issues:
//
// * Barfs on relative file paths
// * Barfs on "--sysroot", replace with "-isysroot"
// * Sometimes barfs on dependency generation commands -M...
// * Complains about unknown compile options, filter some of them out
// * Does not like escaped double quotes, remove escape character
//
// ccache issues:
//
// * Invokes the compiler twice, once with "-E" to get the pre-processor output.
//   The output is suppressed when "-E" is present in the argument list.
//
// * Replaces the output object file with a temporary file, this generates
//   duplicate logs for the same input (if sha1 or arguments is used). The "-o"
//   flag and output file name is therefor removed.
//
func scrubCmdlineRtags(wdir string, args []string) []string {
	scrubbed := make([]string, 0, len(args))
	const (
		stateInitial int = iota
		stateDropNextArg
		stateMakeNextArgRelative
	)

	// Regexp groups
	const (
		singleArgGroup    = 2
		compoundArgGroup1 = 4
		compoundArgGroup2 = 6
		fileArgGroup      = 7
		rspFileArgGroup1  = 9
		rspFileArgGroup2  = 10
	)

	state := stateInitial
next_arg:
	for _, arg := range args {
		if state == stateMakeNextArgRelative {
			scrubbed = append(scrubbed, makeAbsolutePath(wdir, arg))
		}
		if state != stateInitial {
			state = stateInitial
			continue next_arg
		}
		if arg == "-E" {
			// assume ccache pre-processing stage.
			// suppress output.
			return nil
		}
		match := rtagsScrubRegexp.FindStringSubmatch(arg)
		if len(match) == 0 {
			if !rtagsFilterRegexp.MatchString(arg) {
				scrubbed = append(scrubbed, arg)
			}
			continue
		}

		if filename := match[fileArgGroup]; filename != "" {
			scrubbed = append(scrubbed, makeAbsolutePath(wdir, filename))
			continue
		}

		if filename := match[rspFileArgGroup2]; filename != "" {
			// Slurp rsp file and scrub its content.
			rspArgs, err := readRspFile(filename)
			if err != nil {
				scrubbed = append(scrubbed, fmt.Sprintf("-rtags-scrub-error \"%s\"", err))
			} else {
				scrubbed = append(scrubbed, scrubCmdlineRtags(wdir, rspArgs)...)
				continue
			}
		}

		flagArg := match[compoundArgGroup2]
		flag := match[singleArgGroup]
		if flag == "" {
			flag = match[compoundArgGroup1]
		}

		switch flag {
		case "-I", "-isystem":
			if flagArg == "" {
				scrubbed = append(scrubbed, flag)
				state = stateMakeNextArgRelative
			} else {
				scrubbed = append(scrubbed, flag+makeAbsolutePath(wdir, flagArg))
			}
		case "-o", "-MF", "-MQ", "-MT":
			if flagArg == "" {
				state = stateDropNextArg
			}
		case "--sysroot=":
			scrubbed = append(scrubbed, "-isysroot")
			scrubbed = append(scrubbed, makeAbsolutePath(wdir, flagArg))
		case "--sysroot":
			scrubbed = append(scrubbed, "-isysroot")
		}
	}
	return scrubbed
}

func buildRtagsLogentry(wdir, execName string, args []string) []byte {
	b := buildLogentry(execName, scrubCmdlineRtags(wdir, args))
	return bytes.Replace(b, []byte(`\"`), []byte(`"`), -1)
}

func makeAbsolutePath(basedir, path string) string {
	if !filepath.IsAbs(path) {
		if s, err := filepath.Abs(filepath.Join(basedir, path)); err == nil {
			path = s
		}
	}
	return path
}

func writeLogfile(filename string, logentry []byte) error {
	err := os.MkdirAll(filepath.Dir(filename), os.ModePerm)
	if err == nil {
		var file *os.File
		if file, err = os.Create(filename); err == nil {
			_, err = file.Write(logentry)
			file.Close()
		}
	}
	return err
}

// Read CMake rsp file and split it into an argument array.
func readRspFile(filename string) (args []string, err error) {
	var file *os.File
	if file, err = os.Open(filename); err == nil {
		var b []byte
		if b, err = ioutil.ReadAll(file); err == nil {
			// TODO: this does not properly handle quoted arguments with spaces
			for _, b = range bytes.Fields(b) {
				args = append(args, string(b))
			}
		}
	}
	return args, err
}

// Prepend path to application environment search path.
// Returns the new environment.
func prependEnvPath(env []string, path string) []string {
	const varName = "PATH="
	for i, v := range env {
		if strings.HasPrefix(v, varName) {
			if len(v) > len(varName) {
				path = path + ":" + v[len(varName):]
			}
			env[i] = varName + path
			return env
		}
	}
	return append(env, varName+path)
}

// Filter out arguments matching patterns.
// Returns the arguments with matches removed.
func filterOut(args []string, patterns []string) []string {
	var tmp []string
	patternsMap := make(map[string]bool, len(patterns))
	for _, pattern := range patterns {
		patternsMap[pattern] = true
	}
restart:
	for i, v := range args {
		if patternsMap[v] {
			tmp = append(tmp, args[:i]...)
			args = args[i+1:]
			goto restart
		}
	}
	return append(tmp, args...)
}
