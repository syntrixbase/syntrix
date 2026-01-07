package streamer

type Config struct {
	// Service configuration for the Streamer service.
	Service ServiceConfig `yaml:"service"`
	Client  ClientConfig  `yaml:"client"`
}

func DefaultConfig() Config {
	return Config{
		Service: DefaultServiceConfig(),
		Client:  DefaultClientConfig(),
	}
}
