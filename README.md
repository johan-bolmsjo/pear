# Pear

Pear is a compiler interceptor that produce logs to be consumed by the
[RTags](https://github.com/Andersbakken/rtags) code indexer.

RTags has built in support to import compilation logs from tools that can
automatically export them (such as CMake). However there are lots of custom
build systems where it can be hard to automate precise collection of compilation
logs (including all compiler directives and include paths).

The approach taken by this tool is to act as a compiler wrapper. Compilation
commands such as gcc and ld are intercepted and beside performing their primary
action also saves the exact compilation command to a log file that can be
imported into RTags.

Once RTags has learned the compilation flags used to compile a file it manages
itself, automatically re-indexing such files upon change.

## Building the Tool

Pear is written in Go and is compiled by executing `go build` in the repository
root directory.

## Compilation Log Scrubbing

### RTags

RTags had quite some bugs when pear was written so there are some work arounds
in place. I don't know which of these are resolved at the current time.

* Relative paths are converted to absolute paths (such as include directives).
* Replaces `--sysroot` with `-isysroot`.
* Strips dependency generation commands `-MF`, `-MQ`, `-MT`.

### ccache

* Adjusts for the fact that ccache may invoke the compiler twice, once with `-E`
  flag to get the pre-processor output.

* ccache replaces the output object file with a temporary file. This generates
  duplicate logs for the same input if SHA-1 or arguments are used as part of the
  log file name. The `-o` flag and output file name is therefor removed.

## Wrapper Toolchain Commands

A set of wrapper toolchain commands are symlinked to the `pear` program. One
level up from this directory pear will expect to find a configuration file
`pear.conf`. This configuration file contain information about the actual
commands to execute and where (and how) to store compilation logs.

Example Directory Tree:

    host-gcc
    |-- bin
    |   |-- ar -> ../../../pear
    |   |-- as -> ../../../pear
    |   |-- c++ -> ../../../pear
    |   |-- cc -> ../../../pear
    |   |-- c++filt -> ../../../pear
    |   |-- cpp -> ../../../pear
    |   |-- elfedit -> ../../../pear
    |   |-- g++ -> ../../../pear
    |   |-- gcc -> ../../../pear
    |   |-- gcov -> ../../../pear
    |   |-- gprof -> ../../../pear
    |   |-- ld -> ../../../pear
    |   |-- ld.bfd -> ../../../pear
    |   |-- ld.gold -> ../../../pear
    |   |-- nm -> ../../../pear
    |   |-- objcopy -> ../../../pear
    |   |-- objdump -> ../../../pear
    |   |-- ranlib -> ../../../pear
    |   |-- readelf -> ../../../pear
    |   |-- size -> ../../../pear
    |   |-- strings -> ../../../pear
    |   `-- strip -> ../../../pear
    `-- pear.conf

Using the compiler wrapper is just a matter of adjusting the `PATH` environment
variable so that the build system picks up the wrapper commands instead of the
real ones.

## Configuration

The file `pear.config` use a syntax *resembling* JSON. There are two different
section types, environment and command. The environment section define
variables, for example paths to tools that can later be referenced when defining
commands.

Example Environment Section:

    environment: {
        sysroot: /
        binutils: /usr
        gcc:      /usr
    }

The command section may be repeated multiple times. Each section specifies what
commands it applies to and parameters for those commands. A single command is
defined by all command sections that apply settings to it.

Example Command Sections:

    command: {
        name: [ c++ cpp g++ gcc gcov ] 
        exec: @(gcc)/bin/$(.arg0)
    }
    command: {
        name: [ c++ cc cpp g++ gcc ld ld.bfd ld.gold]
        prepend: [ --sysroot @(sysroot) ]
    }
    command: {
        name: [ cc gcc c++ g++ ]
        rtags-logfile: $(.user)/.local/var/rtags/$(.input).$(.arg0).log
    }

The aggregate command for `c++` given the above configuration would be
`/usr/bin/c++ --sysroot /` followed by whatever arguments the wrapper was
invoked with.

The location of RTag compilation logs are specified by the `rtags-logfile`
parameter.

### Environment Variables

Beside the variables defined in the environment section, pear provides some
automatically generated varibles. These variables are all prefixed with a dot.

    .arg0   Command name
    .cdir   Command directory (of wrapper symlink)
    .wdir   Working directory (like pwd)
    .home   $HOME
    .user   $USER
    .date   Date in YYYYMMDD format
    .time   Time in HHMMSS format

The following variables are only valid when referenced by `rtags-logfile` or
`logfile` parameters:

    .sha1    SHA-1 sum of log file content
    .input   Input of compilation command (if available, else SHA-1)
    .output  Output of compilation command (if available, else SHA-1)

An environment variable may be expanded in two ways. The `@(var)` syntax tries
to convert a relative path to an absolute path. The `$(var)` syntax expands a
variable without any modification.

### Command Section Parameters

    name:
        A single command name or a list of command names to which the section
        applies.

    filter-out:
        Filter out the named command arguments before executing the command.

    append:
        Arguments to append to the command argument list.

    prepend:
        Arguments to prepend to the command argument list.

    logfile:
        Path where to write command and arguments that are executed.

    rtags-logfile:
        Path where to write possibly filtered command and arguments for RTags
        consumption

## Complete Example

A complete example is available in the `example/` directory.

The following command sequence would probably utilize the pear wrapper to
intercept the host gcc on a typical Linux host when compiling a hello world C
program.

    go build
    cd example
    ./hello-world.compile

