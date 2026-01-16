package config

import "time"

type Config struct {
	Backends  map[string]BackendConfig  `yaml:"backends"`
	Topology  TopologyConfig            `yaml:"topology"`
	Databases map[string]DatabaseConfig `yaml:"databases"`
}

type DatabaseConfig struct {
	Backend string `yaml:"backend"`
}

type BackendConfig struct {
	Type  string      `yaml:"type"` // "mongo"
	Mongo MongoConfig `yaml:"mongo"`
}

type TopologyConfig struct {
	Document   DocumentTopology   `yaml:"document"`
	User       CollectionTopology `yaml:"user"`
	Revocation CollectionTopology `yaml:"revocation"`
}

type BaseTopology struct {
	Strategy string `yaml:"strategy"` // "single", "read_write_split"
	Primary  string `yaml:"primary"`
	Replica  string `yaml:"replica"`
}

type DocumentTopology struct {
	BaseTopology        `yaml:",inline"`
	DataCollection      string        `yaml:"data_collection"`
	SysCollection       string        `yaml:"sys_collection"`
	SoftDeleteRetention time.Duration `yaml:"soft_delete_retention"`
}

type CollectionTopology struct {
	BaseTopology `yaml:",inline"`
	Collection   string `yaml:"collection"`
}

type MongoConfig struct {
	URI          string `yaml:"uri"`
	DatabaseName string `yaml:"database_name"`
}

func DefaultConfig() Config {
	return Config{
		Backends: map[string]BackendConfig{
			"default_mongo": {
				Type: "mongo",
				Mongo: MongoConfig{
					URI:          "mongodb://localhost:27017",
					DatabaseName: "syntrix",
				},
			},
		},
		Topology: TopologyConfig{
			Document: DocumentTopology{
				BaseTopology: BaseTopology{
					Strategy: "single",
					Primary:  "default_mongo",
				},
				DataCollection:      "documents",
				SysCollection:       "sys",
				SoftDeleteRetention: 5 * time.Minute,
			},
			User: CollectionTopology{
				BaseTopology: BaseTopology{
					Strategy: "single",
					Primary:  "default_mongo",
				},
				Collection: "users",
			},
			Revocation: CollectionTopology{
				BaseTopology: BaseTopology{
					Strategy: "single",
					Primary:  "default_mongo",
				},
				Collection: "revocations",
			},
		},
		Databases: map[string]DatabaseConfig{
			"default": {
				Backend: "default_mongo",
			},
		},
	}
}

func (c *Config) Validate() error {
	return nil
}
