package types

type Args struct {
	Dockerfile string
	Image      string
	Retries    int
}

type DockerConfig struct {
	User         string                    `json:"User"`
	ExposedPorts map[string]map[string]any `json:"ExposedPorts"`
	Env          []string                  `json:"Env"`
	Cmd          []string                  `json:"Cmd"`
	WorkingDir   string                    `json:"WorkingDir"`
	Entrypoint   []string                  `json:"Entrypoint"`
}
