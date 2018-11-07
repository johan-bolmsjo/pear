package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/johan-bolmsjo/pot"
)

type Config struct {
	env Environment
	cmd map[string]*Command
}

type Environment map[string]string

type Command struct {
	name          string
	exec          string
	logfile       string
	rtags_logfile string
	append        []string
	prepend       []string
	filterOut     []string
}

// Create a new configuration.
func NewConfig() *Config {
	return &Config{
		env: make(Environment),
		cmd: make(map[string]*Command),
	}
}

// Read configuration from file
func (config *Config) ReadFile(filename string) error {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	err = config.Parse(buf)
	if err, ok := err.(*pot.ParseError); ok {
		err.Identifier = filename
	}
	return err
}

// Parse configuration from byte slice.
func (config *Config) Parse(buf []byte) (err error) {
	// Line number mapping taking removed comments into account.
	locationAdjustments := make(map[uint32]pot.Location)
	var locationAdjustment pot.Location
	var lineNumber uint32

	// Strip comments (lines beginning with "//").
	// Comments are not parsable by the POT parser.
	ioscanner := bufio.NewScanner(bytes.NewBuffer(buf))
	buf = make([]byte, 0)
	for ioscanner.Scan() {
		line := ioscanner.Bytes()
		if bytes.Index(line, []byte("//")) == 0 {
			locationAdjustment.Line++
		} else {
			buf = append(buf, line...)
			buf = append(buf, '\n')
			locationAdjustments[lineNumber] = locationAdjustment
			lineNumber++
		}
	}
	if err = ioscanner.Err(); err != nil {
		return err
	}

	var key *pot.DictKey
	scanner := pot.NewParserScanner(pot.NewDictParser(buf))
	for scanner.Scan() {
		switch parser := scanner.SubParser().(type) {
		case *pot.DictKey:
			key = parser
		default:
			switch string(key.Bytes()) {
			case "environment":
				err = config.parseEnvironment(parser)
			case "command":
				err = config.parseCommand(parser)
			default:
				err = key.Location().Errorf("unrecognized section name '%s'", string(key.Bytes()))
			}
			scanner.InjectError(err)
		}
	}
	if err = scanner.Err(); err != nil {
		if err, ok := err.(*pot.ParseError); ok {
			locationAdjustment = locationAdjustments[err.Location.Line]
			err.Location.Add(&locationAdjustment)
		}
	}
	return err
}

func (config *Config) GetEnvironment() Environment {
	return config.env
}

func (config *Config) GetCommand(name string) (*Command, error) {
	if cmd := config.cmd[name]; cmd != nil {
		return cmd, nil
	}
	return nil, fmt.Errorf("command '%s' has not been configured")
}

func (config *Config) parseEnvironment(parser pot.Parser) error {
	if _, ok := parser.(*pot.Dict); !ok {
		return parser.Location().Errorf("expected dictionary")
	}
	var keyStr string
	scanner := pot.NewParserScanner(parser)
	for scanner.Scan() {
		switch parser := scanner.SubParser().(type) {
		case *pot.DictKey:
			keyStr = string(parser.Bytes())
		case *pot.String:
			config.env.SetExpanded(keyStr, string(parser.Bytes()))
		default:
			scanner.InjectError(parser.Location().Errorf("expected string"))
		}
	}
	return scanner.Err()
}

func parseListOfStrings(out *[]string, parser pot.Parser) error {
	var err error
	switch parser := parser.(type) {
	case *pot.String:
		*out = append(*out, string(parser.Bytes()))
	case *pot.List:
		scanner := pot.NewParserScanner(parser)
		for scanner.Scan() {
			switch parser := scanner.SubParser().(type) {
			case *pot.String:
				*out = append(*out, string(parser.Bytes()))
			default:
				scanner.InjectError(parser.Location().Errorf("expected string"))
			}
		}
		err = scanner.Err()
	default:
		err = parser.Location().Errorf("expected string or list of strings")
	}
	return err
}

func parseString(out *string, parser pot.Parser) error {
	s, ok := parser.(*pot.String)
	if !ok {
		return parser.Location().Errorf("expected string")
	}
	*out = string(s.Bytes())
	return nil
}

func (config *Config) parseCommand(parser pot.Parser) error {
	if _, ok := parser.(*pot.Dict); !ok {
		return parser.Location().Errorf("expected dictionary")
	}

	var cmdNames []string
	var cmdTmp Command
	var key *pot.DictKey

	scanner := pot.NewParserScanner(parser)
	for scanner.Scan() {
		switch parser := scanner.SubParser().(type) {
		case *pot.DictKey:
			key = parser
		default:
			var err error
			switch string(key.Bytes()) {
			case "append":
				err = parseListOfStrings(&cmdTmp.append, parser)
			case "exec":
				err = parseString(&cmdTmp.exec, parser)
			case "filter-out":
				err = parseListOfStrings(&cmdTmp.filterOut, parser)
			case "logfile":
				err = parseString(&cmdTmp.logfile, parser)
			case "name":
				err = parseListOfStrings(&cmdNames, parser)
			case "prepend":
				err = parseListOfStrings(&cmdTmp.prepend, parser)
			case "rtags-logfile":
				err = parseString(&cmdTmp.rtags_logfile, parser)
			default:
				err = key.Location().Errorf("unrecognized parameter name '%s'", string(key.Bytes()))
			}
			scanner.InjectError(err)
		}
	}

	setString := func(a *string, b string) {
		if b != "" {
			*a = b
		}
	}

	for _, name := range cmdNames {
		// Expand any environment references in the command name
		name = config.env.Expand(name)

		cmd := config.cmd[name]
		if cmd == nil {
			cmd = &Command{name: name}
			config.cmd[name] = cmd
		}
		setString(&cmd.exec, cmdTmp.exec)
		setString(&cmd.logfile, cmdTmp.logfile)
		setString(&cmd.rtags_logfile, cmdTmp.rtags_logfile)
		cmd.append = append(cmd.append, cmdTmp.append...)
		cmd.prepend = append(cmd.prepend, cmdTmp.prepend...)
		cmd.filterOut = append(cmd.filterOut, cmdTmp.filterOut...)
	}
	return scanner.Err()
}
