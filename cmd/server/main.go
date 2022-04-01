package main

import (
	"ephemeralrocket/implementation"
	"ephemeralrocket/util"
)

func main() {
	var config implementation.ServerConfig
	util.ReadJSONConfig("config/server_config.json1", &config)
	server := implementation.NewServer()

	go server.Start(config)

	// block := make(chan int)
	// <-block
}
