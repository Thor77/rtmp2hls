package main

import (
	"github.com/BurntSushi/toml"
	log "github.com/sirupsen/logrus"
)

// Config stores application configuration
type Config struct {
	Addr         string
	Key          string
	MsPerSegment int64
	LogLevel     log.Level
}

func loadConfig(path string) (Config, error) {
	var tmpConfig Config
	if _, err := toml.DecodeFile(path, &tmpConfig); err != nil {
		return tmpConfig, err
	}

	// set default values
	if tmpConfig.Addr == "" {
		tmpConfig.Addr = ":1935"
	}
	if tmpConfig.LogLevel == 0 {
		tmpConfig.LogLevel = log.InfoLevel
	}
	if tmpConfig.MsPerSegment == 0 {
		tmpConfig.MsPerSegment = 15000
	}

	return tmpConfig, nil
}
