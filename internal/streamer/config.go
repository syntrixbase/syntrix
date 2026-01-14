package streamer

type Config struct {
	// Server configuration for the Streamer service.
	Server ServerConfig `yaml:"server"`
	Client ClientConfig `yaml:"client"`
}

func DefaultConfig() Config {
	return Config{
		Server: DefaultServiceConfig(),
		Client: DefaultClientConfig(),
	}
}
