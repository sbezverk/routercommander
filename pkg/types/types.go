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
	Times    int      `yaml:"times"`
	Interval int      `yaml:"interval"`
	Pattern  []string `yaml:"pattern"`
	Debug    bool     `yaml:"debug"`
	RegExp   []*regexp.Regexp
}
type Collect struct {
	HealthCheck bool `yaml:"health_check"`
}
type Commander struct {
	List    []*Command `yaml:"commands"`
	Repro   *Repro     `yaml:"repro"`
	Collect *Collect   `yaml:"collect"`
}
