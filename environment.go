package main

import (
	"path/filepath"
	"regexp"
)

var envExpandRegexp = regexp.MustCompile(`[\$@]\(.+?\)`)

// Expand references to evironment variables using the $(var) syntax in string.
func (env *Environment) Expand(s string) string {
	f := func(m string) string {
		k := m[2 : len(m)-1]
		v := env.Get(k)
		if v == "" {
			return m
		}
		if m[0] == '@' && !filepath.IsAbs(v) {
			if t, err := filepath.Abs(filepath.Join(env.Get(".cdir"), v)); err == nil {
				v = t
			}
		}
		return v
	}
	return envExpandRegexp.ReplaceAllStringFunc(s, f)
}

func (env *Environment) Get(name string) string {
	return (*env)[name]
}

func (env *Environment) Set(name, value string) {
	(*env)[name] = value
}

func (env *Environment) SetExpanded(name, value string) {
	(*env)[name] = env.Expand(value)
}
