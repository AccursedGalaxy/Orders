package config

import (
	"github.com/AccursedGalaxy/Orders/pkg/logger"
	"github.com/spf13/viper"
)

type Config struct {
	Log logger.Config
	// Add other configuration sections here
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
} 