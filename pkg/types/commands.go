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
	var err error
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("fail to unmarshal commands yaml with error: %+v", err)
	}

	pr := false
	if c.Collect != nil {
		pr = c.Collect.ProcessResult
	}
	if c.Repro != nil {
		pr = true
	}
	// Compile Regular Expressions only if Health Check is requested
	for _, cmd := range c.MainCommandGroup {
		if pr || cmd.ProcessResult {
			for _, p := range cmd.Patterns {
				re, err := regexp.Compile(p.PatternString)
				if err != nil {
					return nil, fmt.Errorf("fail to compile regular expression %q with error: %+v", p.PatternString, err)
				}
				p.RegExp = re
			}
		}
		cmd.CommandResult = &CommandResult{
			PatternMatch:  make([]string, 0),
			TriggeredTest: make([]int, 0),
		}
	}

	if len(c.Tests) != 0 {
		c.CommandsWithTests = make(map[string]*Tests)
		for _, t := range c.Tests {
			t.Tests = make(map[int]*Test)
			for i := 0; i < len(t.Source); i++ {
				// Making a map by test id for faster processing
				e := t.Source[i]
				if e.Pattern != nil {
					e.Pattern.RegExp, err = regexp.Compile(e.Pattern.PatternString)
					if err != nil {
						return nil, err
					}
				}
				e.ValuesStore = make(map[int]map[int]interface{})
				t.Tests[t.Source[i].ID] = e
			}
			c.CommandsWithTests[t.Cmd] = t
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
