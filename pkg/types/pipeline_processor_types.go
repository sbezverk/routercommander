package types

type Record map[string]string

type RunContext struct {
	Variables   map[string]string
	Collections map[string][]Record
	Current     Record
	RouterName  string
	Iteration   int
	Depth       int
	StepPath    []string
	MaxDepth    int
	CommandsRun int
	MaxCommands int
}

type Context struct {
	Pattern string `yaml:"pattern"`
}

type Records struct {
	Name          string   `yaml:"name"`
	Pattern       string   `yaml:"pattern"`
	Inherit       []string `yaml:"inherit"`
	DeduplicateBy []string `yaml:"deduplicate_by"`
}

type When struct {
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
}

type Default struct {
	Run *PipelineRun `yaml:"run,omitempty"`
}

type PipelineCase struct {
	When  *When           `yaml:"when"`
	Steps []*PipelineStep `yaml:"then"`
}

type PipelineBranch struct {
	Cases   []*PipelineCase `yaml:"cases"`
	Default []*PipelineStep `yaml:"default"`
}

type PipelineExtract struct {
	Records *Records   `yaml:"records"`
	Context []*Context `yaml:"context"`
}

type PipelineForEach struct {
	In    string          `yaml:"in"`
	Steps []*PipelineStep `yaml:"steps"`
}

type PipelineRun struct {
	Command *Command `yaml:"command"`
}

type PipelineStep struct {
	ID      string           `yaml:"id"`
	Run     *Command         `yaml:"run,omitempty"`
	Extract *PipelineExtract `yaml:"extract,omitempty"`
	ForEach *PipelineForEach `yaml:"for_each,omitempty"`
	Branch  *PipelineBranch  `yaml:"branch,omitempty"`
	OnError string           `yaml:"on_error,omitempty"`
}
