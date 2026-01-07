package config

type Config struct {
	Port int `yaml:"port"`
}

func DefaultConfig() Config {
	return Config{
		Port: 8083,
	}
}
