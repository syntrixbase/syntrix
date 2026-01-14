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
