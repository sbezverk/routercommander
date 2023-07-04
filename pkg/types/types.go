package types

import "regexp"

type Command struct {
	Cmd           string     `yaml:"command"`
	CmdTimeout    int        `yaml:"command_timeout"`
	Times         int        `yaml:"times"`
	Interval      int        `yaml:"interval"`
	WaitBefore    int        `yaml:"wait_before"`
	WaitAfter     int        `yaml:"wait_after"`
	Location      []string   `yaml:"location"`
	Debug         bool       `yaml:"debug"`
	ProcessResult bool       `yaml:"process_result"`
	Patterns      []*Pattern `yaml:"patterns"`
	// TestID used to logically connect the command
	// from main_command_group to specific set of tests
	// defined in tests section for a specific command. If TestIDs are not specified
	// then all tests defined for a specific command are executed.
	TestIDs       []int `yaml:"command_test_ids"`
	CommandResult *CommandResult
}

type Commander struct {
	Repro             *Repro     `yaml:"repro"`
	Collect           *Collect   `yaml:"collect"`
	Tests             []*Tests   `yaml:"tests"`
	MainCommandGroup  []*Command `yaml:"commands"`
	CommandsWithTests map[string]*Tests
}

type Repro struct {
	Times                  int        `yaml:"times"`
	Interval               int        `yaml:"interval"`
	PostMortemCommandGroup []*Command `yaml:"if_triggered_commands"`
	StopWhenTriggered      bool       `yaml:"stop_when_triggered"`
}

type Collect struct {
	ProcessResult bool `yaml:"process_result"`
}

type Tests struct {
	Cmd    string  `yaml:"command"`
	Source []*Test `yaml:"command_tests"`
	Tests  map[int]*Test
}

type Test struct {
	ID                  int        `yaml:"id"`
	Pattern             *Pattern   `yaml:"pattern"`
	Occurrence          int        `yaml:"occurrence"`
	NumberOfOccurences  *int       `yaml:"number_of_occurrences"`
	Fields              []*Field   `yaml:"fields"`
	Separator           string     `yaml:"separator"`
	IfTriggeredCommands []*Command `yaml:"if_triggered_commands"`
	CheckAllResults     bool       `yaml:"check_all_results"`
	ValuesStore         map[int]map[int]interface{}
}

type Field struct {
	FieldNumber int    `yaml:"field_number"`
	Operation   string `yaml:"operation"`
	Value       string `yaml:"value"`
	Result      interface{}
}

type Pattern struct {
	PatternString string `yaml:"pattern_string"`
	RegExp        *regexp.Regexp
}

type CommandResult struct {
	PatternMatch  map[string][]string
	TriggeredTest []int
}
