package dccli

type Config struct {
	Version  string              `yaml:"version,omitempty"`
	Networks map[string]*Network `yaml:"networks,omitempty"`
	//Volumes  map[string]volume  `yaml:"volumes"`
	Services map[string]Service `yaml:"services,omitempty"`
}

type Network struct {
	Driver   string
	External string
	//DriverOpts map[string]string "driver_opts"
}

//type volume struct {
//	Driver, External string
//	DriverOpts       map[string]string "driver_opts"
//}

type Service struct {
	//ContainerName string   `yaml:"container_name,omitempty"`
	Image      string   `yaml:"image,omitempty"`
	Entrypoint string   `yaml:"entrypoint,omitempty"`
	Networks   []string `yaml:"networks,omitempty"`
	//Expose        []string    `yaml:"expose,omitempty"`
	Hostname    string            `yaml:"hostname,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	HealthCheck HealthCheck       `yaml:"healthcheck,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
}

type HealthCheck struct {
	Test        []string `yaml:"test,omitempty"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
	Retries     string   `yaml:"retries,omitempty"`
}
