package types

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v2"
)

func readCommandFile(fn string) ([]byte, error) {
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
	return b, nil
}

func parseCommandFile(b []byte) (*Commander, error) {
	c := &Commander{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("fail to unmarshal commands yaml with error: %+v", err)
	}

	// TODO (sbezverk) Add logic validation of parameters

	hc := false
	if c.Collect != nil {
		hc = c.Collect.HealthCheck
	}
	if c.Repro != nil {
		hc = true
	}
	// Compile Regular Expressions only if Health Check is requested
	for _, cmd := range c.MainCommandGroup {
		if hc || cmd.ProcessResult {
			for _, p := range cmd.Patterns {
				re, err := regexp.Compile(p.PatternString)
				if err != nil {
					return nil, fmt.Errorf("fail to compile regular expression %q with error: %+v", p.PatternString, err)
				}
				p.RegExp = re
			}
		}
	}
	// Populating Special Captured Values Processing  map
	if c.Repro != nil {
		// First Key is command, second Key is test ID for the command
		c.Repro.CommandTests = make(map[string]map[int]*CommandTest)
		for _, cpr := range c.Repro.CommandProcessingRules {
			c.Repro.CommandTests[cpr.Cmd] = make(map[int]*CommandTest)
			for _, p := range cpr.Tests {
				if p.Pattern != nil {
					p.Pattern.RegExp = regexp.MustCompile(p.Pattern.PatternString)
				}
				p.ValuesStore = make(map[int]map[int]interface{})
				c.Repro.CommandTests[cpr.Cmd][p.ID] = p

			}
		}
	}

	return c, nil

}

func GetCommands(fn string) (*Commander, error) {
	b, err := readCommandFile(fn)
	if err != nil {
		return nil, err
	}

	return parseCommandFile(b)
}
