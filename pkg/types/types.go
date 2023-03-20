package types

import "regexp"

type Command struct {
	Cmd           string     `yaml:"command"`
	Times         int        `yaml:"times"`
	Interval      int        `yaml:"interval"`
	WaitBefore    int        `yaml:"wait_before"`
	WaitAfter     int        `yaml:"wait_after"`
	Location      []string   `yaml:"location"`
	Debug         bool       `yaml:"debug"`
	ProcessResult bool       `yaml:"process_result"`
	Patterns      []*Pattern `yaml:"patterns"`
}

type Repro struct {
	Times                  int        `yaml:"times"`
	Interval               int        `yaml:"interval"`
	CommandProcessingRules []*Command `yaml:"command_processing_rules"`
	PostMortemCommandGroup []*Command `yaml:"postmortem_command_group"`
}
type Collect struct {
	HealthCheck bool `yaml:"health_check"`
}
type Commander struct {
	MainCommandGroup []*Command `yaml:"main_command_group"`
	Repro            *Repro     `yaml:"repro"`
	Collect          *Collect   `yaml:"collect"`
}

type CapturedValue struct {
	FieldNumber int    `yaml:"field_number"`
	Operation   string `yaml:"operation"`
	Result      interface{}
}
type Pattern struct {
	PatternString            string           `yaml:"pattern_string"`
	Capture                  *Capture         `yaml:"capture"`
	CapturedValuesProcessing []*CapturedValue `yaml:"captured_values"`
	PatternCommands          []*Command       `yaml:"pattern_commands"`
	CheckAllResults          bool             `yaml:"check_all_results"`
	RegExp                   *regexp.Regexp
	Values                   map[int]map[int]interface{}
}

type Capture struct {
	FieldNumber []int  `yaml:"field_number"`
	Separator   string `yaml:"separator"`
}
