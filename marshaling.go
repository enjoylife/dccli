package dccli

import (
	"fmt"
	"strings"
)

type Config struct {
	Version   string                 `yaml:"version,omitempty"`
	Networks  map[string]*Network    `yaml:"networks,omitempty"`
	Volumes   map[string]interface{} `yaml:"volumes,omitempty"`
	Services  map[string]Service     `yaml:"services,omitempty"`
	Extension map[string]interface{} `yaml:",inline,omitempty"`
}

type Network struct {
	Driver   string
	External string
	//DriverOpts map[string]string "driver_opts"
	Extension map[string]interface{} `yaml:",inline,omitempty"`
}

type Volume struct {
	Source     string                 `yaml:"source,omitempty"`
	Target     string                 `yaml:"target,omitempty"`
	Driver     string                 `yaml:"driver,omitempty"`
	External   string                 `yaml:"external,omitempty"`
	Type       string                 `yaml:"type,omitempty"`
	DriverOpts map[string]string      `yaml:"services,omitempty"`
	Volume     map[string]interface{} `yaml:"volume,omitempty"`
}

type volToMarshal struct {
	Source     string                 `yaml:"source,omitempty"`
	Target     string                 `yaml:"target,omitempty"`
	Driver     string                 `yaml:"driver,omitempty"`
	External   string                 `yaml:"external,omitempty"`
	Type       string                 `yaml:"type,omitempty"`
	DriverOpts map[string]string      `yaml:"services,omitempty"`
	Volume     map[string]interface{} `yaml:"volume,omitempty"`
}

func (v *Volume) UnmarshalYAML(unmarshal func(interface{}) error) error {

	// try to unmarshal into the old style first, allowing
	var old string
	if err := unmarshal(&old); err == nil {
		strs := strings.Split(old, ":")
		if len(strs) != 2 {
			return fmt.Errorf("invalid format: %s", old)
		}
		v.Source = strs[0]
		v.Target = strs[1]
		return nil
	}

	vCopy := volToMarshal{}
	if err := unmarshal(&vCopy); err == nil {
		*v = Volume(vCopy)
		return nil
	}
	return fmt.Errorf("could not unmarshal into volumes")
}

//func (v Volume) MarshalYAML() (interface{}, error) {
//	if v.oldStyle != "" {
//		return v.oldStyle, nil
//	}
//	return v.VolumeLongSyntax, nil
//}

type Service struct {
	//ContainerName string   `yaml:"container_name,omitempty"`
	Image      string   `yaml:"image,omitempty"`
	Entrypoint string   `yaml:"entrypoint,omitempty"`
	Networks   []string `yaml:"networks,omitempty"`
	//Expose        []string    `yaml:"expose,omitempty"`
	Hostname    string                 `yaml:"hostname,omitempty"`
	Ports       []string               `yaml:"ports,omitempty"`
	Volumes     []*Volume              `yaml:"volumes,omitempty"`
	Command     []string               `yaml:"command,omitempty"`
	HealthCheck HealthCheck            `yaml:"healthcheck,omitempty"`
	DependsOn   []string               `yaml:"depends_on,omitempty"`
	Environment []string               `yaml:"environment,omitempty"`
	Deploy      map[string]interface{} `yaml:"deploy,omitempty"`
	Extension   map[string]interface{} `yaml:",inline,omitempty"`
}

type RestartPolicy struct {
	Condition   string `yaml:"condition,omitempty"`
	Delay       string `yaml:"delay,omitempty"`
	MaxAttempts int    `yaml:"max_attempts,omitempty"`
	Window      string `yaml:"window,omitempty"`
}

type HealthCheck struct {
	Test        []string `yaml:"test,omitempty"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
	Retries     string   `yaml:"retries,omitempty"`
}
