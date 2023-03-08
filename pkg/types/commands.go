package types

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v2"
)

func GetCommands(fn string) (*Commander, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, fmt.Errorf("fail to open file %s with error: %+v", fn, err)
	}
	defer f.Close()
	l, err := os.Stat(fn)
	if err != nil {
		return nil, fmt.Errorf("fail to get stats for file %s with error: %+v", fn, err)
	}
	b := make([]byte, l.Size())
	if _, err := f.Read(b); err != nil {
		return nil, fmt.Errorf("fail to read file %s with error: %+v", fn, err)
	}
	c := &Commander{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("fail tp unmarshal commands yaml file %s with error: %+v", fn, err)
	}

	// TODO (sbezverk) Add logic validation of parameters

	hc := false
	if c.Collect != nil {
		hc = c.Collect.HealthCheck
	}
	// Compile Regular Expressions only if Health Check is requested
	if hc {
		for _, cmd := range c.List {
			cmd.RegExp = make([]*regexp.Regexp, len(cmd.Pattern))
			for i, p := range cmd.Pattern {
				cmd.RegExp[i], err = regexp.Compile(p)
				if err != nil {
					return nil, fmt.Errorf("fail to compile regular expression %q with error: %+v", p, err)
				}
			}
		}
	}

	// TODO (sbezverk) Add processing patterns for repro section

	return c, nil
}
