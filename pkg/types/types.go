package types

import "regexp"

type Command struct {
	Cmd        string   `yaml:"command"`
	Times      int      `yaml:"times"`
	Interval   int      `yaml:"interval"`
	WaitBefore int      `yaml:"wait_before"`
	WaitAfter  int      `yaml:"wait_after"`
	Location   []string `yaml:"location"`
	Pattern    []string `yaml:"pattern"`
	Debug      bool     `yaml:"debug"`
	RegExp     []*regexp.Regexp
}

type Repro struct {
	Times    int `yaml:"times"`
	Interval int `yaml:"interval"`
}
type Commander struct {
	Mode  string     `yaml:"mode"`
	List  []*Command `yaml:"commands"`
	Repro *Repro     `yaml:"repro"`
}
