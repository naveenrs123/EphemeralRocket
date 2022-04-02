package main

import (
	"ephemeralrocket/implementation"
	"ephemeralrocket/util"
	"os"
)

func main() {
	var config implementation.ServerConfig
	configPath := os.Getenv("SERVER_CONFIG")
	// util.ReadJSONConfig("config/server_config1.json", &config) //Uncomment to set configs mannually
	util.ReadJSONConfig(configPath, &config)
	server := implementation.NewServer()
	server.Start(config)
}
