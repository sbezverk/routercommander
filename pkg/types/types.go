package types

import "regexp"

type Command struct {
	Cmd           string     `yaml:"command"`
	Times         int        `yaml:"times"`
	Interval      int        `yaml:"interval"`
	WaitBefore    int        `yaml:"wait_before"`
	WaitAfter     int        `yaml:"wait_after"`
	Location      []string   `yaml:"location"`
	Pattern       []string   `yaml:"pattern"`
	Debug         bool       `yaml:"debug"`
	CollectResult bool       `yaml:"collect_result"`
	Patterns      []*Pattern `yaml:"patterns"`
	RegExp        []*regexp.Regexp
}

type Repro struct {
	Times          int        `yaml:"times"`
	Interval       int        `yaml:"interval"`
	PostMortemList []*Command `yaml:"commands"`
}
type Collect struct {
	HealthCheck bool `yaml:"health_check"`
}
type Commander struct {
	List    []*Command `yaml:"commands"`
	Repro   *Repro     `yaml:"repro"`
	Collect *Collect   `yaml:"collect"`
}

type Pattern struct {
	PatternString string   `yaml:"pattern_string"`
	Capture       *Capture `yaml:"capture"`
	RegExp        *regexp.Regexp
}

type Capture struct {
	FieldNumber int    `yaml:"field_number"`
	Separator   string `yaml:"separator"`
	Value       interface{}
}
