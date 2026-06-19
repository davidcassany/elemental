package dynamicservice

const (
	DefaultK8sDynamicConfigPath = "/var/lib/elemental/k8s-dynamic/userdata.yaml"
	DefaultK8sDynamicTimeout    = 120
)

type Config struct {
	Services Services `yaml:"services,omitempty" json:"services,omitempty"`
}

type Services struct {
	K8sDynamic Service `yaml:"k8s-dynamic,omitempty" json:"k8s-dynamic,omitempty"`
}

type Service struct {
	Enabled bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Config  string `yaml:"config,omitempty" json:"config,omitempty"`
	Timeout int    `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

func (c *Config) Default() {
	if c.Services.K8sDynamic.Enabled {
		if c.Services.K8sDynamic.Config == "" {
			c.Services.K8sDynamic.Config = DefaultK8sDynamicConfigPath
		}
		if c.Services.K8sDynamic.Timeout <= 0 {
			c.Services.K8sDynamic.Timeout = DefaultK8sDynamicTimeout
		}
	}
}

func (c Config) K8sDynamicEnabled() bool {
	return c.Services.K8sDynamic.Enabled
}
