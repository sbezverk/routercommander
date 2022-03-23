package types

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

func GetCommands(fn string) (*Commands, error) {
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
	c := &Commands{
		List: make([]*ShowCommand, 0),
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("fail tp unmarshal commands yaml file %s with error: %+v", fn, err)
	}

	return c, nil
}
