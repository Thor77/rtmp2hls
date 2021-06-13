package main

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	log "github.com/sirupsen/logrus"
)

// Config stores application configuration
type Config struct {
	Addr         string
	Key          string
	MsPerSegment int64
	LogLevel     log.Level
	HLSDirectory string
	BaseURL      string
}

func loadConfig(path string) (Config, error) {
	log.Debugf("Reading config from %s\n", path)

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
	if tmpConfig.HLSDirectory == "" {
		tmpConfig.HLSDirectory = "."
	} else if _, err := os.Stat(tmpConfig.HLSDirectory); err != nil {
		// try to create dir
		log.Debugln("HLS directory doesn't exist, trying to create it")
		if err := os.Mkdir(tmpConfig.HLSDirectory, 0755); err != nil {
			return Config{}, fmt.Errorf("error while checking hls directory: %v", err)
		}
	}

	return tmpConfig, nil
}
