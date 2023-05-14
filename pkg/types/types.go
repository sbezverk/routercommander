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
	// Tests used in Repro mode
	Tests []*CommandTest `yaml:"tests"`
	// TestID used to logically connect the command
	// from main_command_group to specific set of tests
	// defined in repro mode
	TestID int `yaml:"command_test_id"`
}

type Repro struct {
	Times                  int        `yaml:"times"`
	Interval               int        `yaml:"interval"`
	CommandProcessingRules []*Command `yaml:"command_processing_rules"`
	PostMortemCommandGroup []*Command `yaml:"postmortem_command_group"`
	// CommandTests defines map of commands, the key is command,
	// the next level is map of tests, the key is test id
	CommandTests map[string]map[int]*CommandTest
	// PerCmdPerPatternCommands map[string]map[string][]*Command
}

type Collect struct {
	HealthCheck bool `yaml:"health_check"`
}

type Commander struct {
	MainCommandGroup []*Command `yaml:"main_command_group"`
	Repro            *Repro     `yaml:"repro"`
	Collect          *Collect   `yaml:"collect"`
}

type Field struct {
	FieldNumber int    `yaml:"field_number"`
	Operation   string `yaml:"operation"`
	Value       string `yaml:"value"`
	Result      interface{}
}

type CommandTest struct {
	ID                  int        `yaml:"id"`
	Pattern             *Pattern   `yaml:"pattern"`
	Occurrence          int        `yaml:"occurrence"`
	NumberOfOccurences  *int       `yaml:"number_of_occurrences"`
	Fields              []*Field   `yaml:"fields"`
	Separator           string     `yaml:"separator"`
	IfTriggeredCommands []*Command `yaml:"if_triggered_commands"`
	CheckAllResults     bool       `yaml:"check_all_results"`
	RegExp              *regexp.Regexp
	ValuesStore         map[int]map[int]interface{}
}

type Pattern struct {
	PatternString string `yaml:"pattern_string"`
	RegExp        *regexp.Regexp
}
