package gitlab

type Config struct {
	Application *Gitlab `yaml:"application"`
	Cluster     *Gitlab `yaml:"cluster"`
}

type Gitlab struct {
	GitlabName      string  `yaml:"gitlabName"`
	Parent          *Parent `yaml:"parent"`
	RecyclingParent *Parent `yaml:"recyclingParent"`
}

type Parent struct {
	Path string `yaml:"path"`
	ID   int    `yaml:"id"`
}
