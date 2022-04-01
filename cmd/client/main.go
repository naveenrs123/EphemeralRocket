package main

import (
	"ephemeralrocket/implementation"
	"ephemeralrocket/messagelib"
	"ephemeralrocket/util"
	"fmt"
)

func main() {
	var config implementation.ClientConfig
	err := util.ReadJSONConfig("config/client_config.json", &config)
	client := messagelib.NewMessageLib()
	messageChan, err := client.Start(config)
	util.CheckErr(err, "Error starting messagelib:")
	outer: for {
		select {
		case msg := <-messageChan:
			fmt.Println("received message")
			fmt.Println("Fetched new message")
			fmt.Println(msg)
		default:
			fmt.Println("default")
			clientIds := client.ViewClients()
			fmt.Println(clientIds)
			break outer
		}
	}

	fmt.Println("This is the client!")
}
