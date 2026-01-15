package config

type Config struct {
	NatsURL            string `yaml:"nats_url"`
	RulesFile          string `yaml:"rules_file"`
	WorkerCount        int    `yaml:"worker_count"`
	StreamName         string `yaml:"stream_name"`
	CheckpointDatabase string `yaml:"checkpoint_database"`
}

func DefaultConfig() Config {
	return Config{
		NatsURL:            "nats://localhost:4222",
		RulesFile:          "triggers.json",
		WorkerCount:        16,
		CheckpointDatabase: "default",
	}
}
