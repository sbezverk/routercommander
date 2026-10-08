package types

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

type Command struct {
	Cmd                string             `yaml:"command"`
	templatedCmd       *template.Template // Cached command template used for every command execution.
	CmdTimeout         int                `yaml:"command_timeout"`
	Times              int                `yaml:"times"`
	Interval           int                `yaml:"interval"`
	WaitBefore         int                `yaml:"wait_before"`
	WaitAfter          int                `yaml:"wait_after"`
	Location           []string           `yaml:"location"`
	LocationFmtTmpl    string             `yaml:"location_fmt_tmpl"` // node0_{{.Slot}}_cpu0, default 0/0/cpu0
	templatedLocation  *template.Template
	LocationCustomized bool       `yaml:"location_customized"` // position of location is defined by a variable {{.Location}} in a command
	PipeModifier       string     `yaml:"pipe_modifier"`
	Debug              bool       `yaml:"debug"`
	ProcessResult      bool       `yaml:"process_result"`
	Patterns           []*Pattern `yaml:"patterns"`
	// TestID used to logically connect the command
	// from commands to specific set of tests
	// defined in tests section for a specific command. If TestIDs are not specified
	// then all tests defined for a specific command are executed.
	TestIDs           []int `yaml:"command_test_ids"`
	CommandResult     *CommandResult
	runtimeRenderData map[string]any
}

func (cmd *Command) SetRuntimeRenderData(data map[string]any) {
	cmd.runtimeRenderData = data
}

func (cmd *Command) Validate() error {
	if cmd == nil {
		return fmt.Errorf("command is nil")
	}
	if strings.TrimSpace(cmd.Cmd) == "" {
		return fmt.Errorf("command string is empty")
	}
	tmpl, err := template.New("cmd").Option("missingkey=error").Parse(cmd.Cmd)
	if err != nil {
		return fmt.Errorf("command template parsing failed: %v", err)
	}
	cmd.templatedCmd = tmpl
	if cmd.CmdTimeout < 0 {
		return fmt.Errorf("command timeout cannot be negative")
	}
	if cmd.Times < 0 {
		return fmt.Errorf("command times cannot be negative")
	}
	if cmd.Interval < 0 {
		return fmt.Errorf("command interval cannot be negative")
	}
	if cmd.WaitBefore < 0 {
		return fmt.Errorf("command wait_before cannot be negative")
	}
	if cmd.WaitAfter < 0 {
		return fmt.Errorf("command wait_after cannot be negative")
	}
	if err := validateLocations(cmd.Location); err != nil {
		return err
	}
	cmd.templatedLocation = nil
	if cmd.LocationFmtTmpl != "" {
		tmpl, err := template.New("location").Option("missingkey=error").Parse(cmd.LocationFmtTmpl)
		if err != nil {
			return fmt.Errorf("location format template parsing failed: %v", err)
		}
		cmd.templatedLocation = tmpl
	}
	for _, pattern := range cmd.Patterns {
		if err := pattern.Compile(); err != nil {
			return err
		}
	}

	return nil
}

// Render executes the command template with the supplied data. Command text
// is always treated as a text/template, including commands without template
// expressions, so callers use one rendering path for static and dynamic
// commands alike.
func (cmd *Command) Render(data any) (string, error) {
	if cmd == nil {
		return "", fmt.Errorf("command is nil")
	}
	if strings.TrimSpace(cmd.Cmd) == "" {
		return "", fmt.Errorf("command string is empty")
	}
	if cmd.templatedCmd == nil {
		tmpl, err := template.New("cmd").Option("missingkey=error").Parse(cmd.Cmd)
		if err != nil {
			return "", fmt.Errorf("command template parsing failed: %v", err)
		}
		cmd.templatedCmd = tmpl
	}
	var rendered bytes.Buffer
	if err := cmd.templatedCmd.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("command template execution failed: %w", err)
	}
	return rendered.String(), nil
}

// ValidateRendered verifies command text after template expansion. It is
// intentionally limited to platform-independent invalid values; platform
// syntax and quoting remain the responsibility of the router implementation.
func (cmd *Command) ValidateRendered(rendered string) error {
	if strings.TrimSpace(rendered) == "" {
		return fmt.Errorf("rendered command is empty")
	}
	if strings.ContainsRune(rendered, '\x00') {
		return fmt.Errorf("rendered command contains a NUL character")
	}
	if strings.ContainsAny(rendered, "\r\n") {
		return fmt.Errorf("rendered command contains a newline")
	}

	return nil
}

func validateLocations(locations []string) error {
	if locations == nil {
		return nil
	}
	for _, l := range locations {
		if strings.TrimSpace(l) == "" {
			return fmt.Errorf("location cannot be empty string")
		}
	}

	return nil
}

type Commander struct {
	Repro             *Repro          `yaml:"repro"`
	Collect           *Collect        `yaml:"collect"`
	Tests             []*Tests        `yaml:"tests"`
	MainCommandGroup  []*Command      `yaml:"commands"`
	Pipeline          []*PipelineStep `yaml:"pipeline"`
	PipelineLimits    *PipelineLimits `yaml:"pipeline_limits"`
	CommandsWithTests map[string]*Tests
}

func (c *Commander) Validate() error {
	if c == nil {
		return fmt.Errorf("commander is nil")
	}

	if len(c.MainCommandGroup) != 0 && len(c.Pipeline) != 0 {
		return fmt.Errorf("both MainCommandGroup and Pipeline are defined, only one is allowed")
	}
	if len(c.MainCommandGroup) == 0 && len(c.Pipeline) == 0 {
		return fmt.Errorf("neither MainCommandGroup nor Pipeline is defined, one is required")
	}
	if len(c.MainCommandGroup) != 0 {
		for _, cmd := range c.MainCommandGroup {
			if cmd == nil {
				return fmt.Errorf("command in MainCommandGroup is nil")
			}
			if err := cmd.Validate(); err != nil {
				return fmt.Errorf("command %q validation failed: %v", cmd.Cmd, err)
			}
			cmd.CommandResult = &CommandResult{
				PatternMatch:  make([]string, 0),
				TriggeredTest: make([]int, 0),
			}
		}

		if c.Repro != nil {
			for _, cmd := range c.Repro.PostMortemCommandGroup {
				if err := cmd.Validate(); err != nil {
					return fmt.Errorf("post-mortem command %q validation failed: %v", cmd.Cmd, err)
				}
			}
		}
		if len(c.Tests) != 0 {
			c.CommandsWithTests = make(map[string]*Tests)
			for _, t := range c.Tests {
				t.Tests = make(map[int]*Test)
				for i := 0; i < len(t.Source); i++ {
					// Making a map by test id for faster processing
					e := t.Source[i]
					if e == nil {
						return fmt.Errorf("test for command %q is nil", t.Cmd)
					}
					for _, cmd := range e.IfTriggeredCommands {
						if err := cmd.Validate(); err != nil {
							return fmt.Errorf("test ID %d for command %q validation failed: %v", e.ID, t.Cmd, err)
						}
					}
					if e.Pattern != nil {
						if err := e.Pattern.Compile(); err != nil {
							return fmt.Errorf("fail to compile pattern for test ID %d with error: %+v", e.ID, err)
						}
					}
					e.ValuesStore = make(map[int]map[int]interface{})
					t.Tests[t.Source[i].ID] = e
				}
				c.CommandsWithTests[t.Cmd] = t
			}
		}
	} else {
		if c.PipelineLimits == nil {
			c.PipelineLimits = &PipelineLimits{
				MaxDepth:    DefaultMaxPipelineDepth,
				MaxCommands: DefaultMaxPipelineCommands,
				MaxRecords:  DefaultMaxRecords,
			}
		} else {
			if c.PipelineLimits.MaxDepth <= 0 {
				return fmt.Errorf("pipeline max depth must be greater than 0")
			} else if c.PipelineLimits.MaxDepth > MaximumPipelineDepth {
				return fmt.Errorf("pipeline max depth cannot exceed %d", MaximumPipelineDepth)
			}
			if c.PipelineLimits.MaxCommands <= 0 {
				return fmt.Errorf("pipeline max commands must be greater than 0")
			} else if c.PipelineLimits.MaxCommands > MaximumPipelineCommands {
				return fmt.Errorf("pipeline max commands cannot exceed %d", MaximumPipelineCommands)
			}
			if c.PipelineLimits.MaxRecords < 0 {
				return fmt.Errorf("pipeline max records must be greater or equal to 0")
			} else if c.PipelineLimits.MaxRecords > MaximumRecords {
				return fmt.Errorf("pipeline max records cannot exceed %d", MaximumRecords)
			}
		}
		// Processing Pipeline
		if err, pSteps := validatePipeline(c.Pipeline, &pipelineValidationContext{
			inForEachStep:   false,
			stepIDs:         make(map[string]struct{}),
			collectionNames: make(map[string]struct{}),
			pipelineSymbols: pipelineSymbols{
				contextFields:    make(map[string]struct{}),
				collectionFields: make(map[string]map[string]struct{}),
				variableFields:   make(map[string]struct{}),
			},
			levels:           0,
			maxPipelineDepth: c.PipelineLimits.MaxDepth,
		}); err != nil {
			return err
		} else {
			c.Pipeline = pSteps
		}
	}

	return nil
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

func (p *Pattern) Compile() error {
	if p == nil {
		return fmt.Errorf("pattern is nil")
	}
	if strings.TrimSpace(p.PatternString) == "" {
		return fmt.Errorf("pattern string is empty")
	}
	re, err := regexp.Compile(p.PatternString)
	if err != nil {
		return err
	}
	p.RegExp = re
	return nil
}

type CommandResult struct {
	PatternMatch  []string
	TriggeredTest []int
}

type PipelineLimits struct {
	MaxDepth    int `yaml:"max_depth"`    // Maximum recursive pipeline depth.
	MaxCommands int `yaml:"max_commands"` // Maximum generated commands per router.
	MaxRecords  int `yaml:"max_records"`  // Maximum permitted records in a collection.
}

type pipelineSymbols struct {
	contextFields    map[string]struct{}
	collectionFields map[string]map[string]struct{}
	variableFields   map[string]struct{}
}

type pipelineValidationContext struct {
	inForEachStep    bool
	stepIDs          map[string]struct{}
	collectionNames  map[string]struct{}
	pipelineSymbols  pipelineSymbols
	levels           int
	maxPipelineDepth int
}
