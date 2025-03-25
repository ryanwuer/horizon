package pipeline

type Config struct {
	// DisableApproval is a flag to disable approval of the pipeline.
	DisableApproval bool `yaml:"disableApproval"`
}
