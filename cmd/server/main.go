package main

import (
	"ephemeralrocket/implementation"
	"ephemeralrocket/util"
)

func main() {
	var config implementation.ServerConfig
	util.ReadJSONConfig("config/server_config1.json", &config)
	server := implementation.NewServer()

	server.Start(config)

	// block := make(chan int)
	// <-block
}
