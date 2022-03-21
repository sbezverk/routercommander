package types

type ShowCommand struct {
	Cmd      string   `yaml:"command"`
	Times    int      `yaml:"times"`
	Interval int      `yaml:"interval"`
	Location []string `yaml:"location"`
	Pattern  []string `yaml:"pattern"`
}

type Commands struct {
	List []*ShowCommand `yaml:"commands"`
}
