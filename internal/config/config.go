package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Observability ObservabilityConfig `mapstructure:"observability"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

type ObservabilityConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	ServiceName    string `mapstructure:"service_name"`
	OTLPEndpoint   string `mapstructure:"otlp_endpoint"`
	MetricsEnabled bool   `mapstructure:"metrics_enabled"`
	TracesEnabled  bool   `mapstructure:"traces_enabled"`
}

func Load() *Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("configs")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("Unable to decode into struct: %v", err)
	}

	return &config
}