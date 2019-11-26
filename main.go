package main

import (
	"github.com/nareix/joy4/format/rtmp"
	"os"

	log "github.com/sirupsen/logrus"
)

var config Config

func main() {
	var err error

	configPath := "config.toml"
	if len(os.Args) >= 2 {
		configPath = os.Args[1]
	}

	config, err = loadConfig(configPath)
	if err != nil {
		log.Fatalln(err)
	}

	// setup logger
	log.SetLevel(config.LogLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	// initialize server
	server := &rtmp.Server{Addr: config.Addr}
	server.HandlePublish = publishHandler

	log.Infof("Listening on %s\n", server.Addr)
	server.ListenAndServe()
}
