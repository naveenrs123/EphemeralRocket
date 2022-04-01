package main

import (
	"ephemeralrocket/implementation"
	"ephemeralrocket/util"
)

func main() {
	var config implementation.CoordConfig
	util.ReadJSONConfig("config/coord_config.json", &config)
	coord := implementation.NewCoord()
	coord.Start(config)
}
