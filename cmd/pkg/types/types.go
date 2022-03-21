package types

type ShowCommand struct {
	Times    int      `yaml:"times"`
	Interval int      `yaml:"interval"`
	Location []string `yaml:"location"`
	Pattern  []string `yaml:"pattern"`
}

type Commands struct {
	List map[string]*ShowCommand `yaml:"commands"`
}
