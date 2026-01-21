package config

type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Database       DatabaseConfig       `mapstructure:"database"`
	Observability  ObservabilityConfig  `mapstructure:"observability"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

type ObservabilityConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	ServiceName string `mapstructure:"service_name"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
	MetricsEnabled bool `mapstructure:"metrics_enabled"`
	TracesEnabled  bool `mapstructure:"traces_enabled"`
}