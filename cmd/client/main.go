package main

import (
	"bufio"
	"ephemeralrocket/implementation"
	"ephemeralrocket/messagelib"
	"ephemeralrocket/util"
	"fmt"
	"os"
)

var actions []string = []string{"View Messages", "Start a new converstaion", "Send Message", "quit"}

var MesMap map[uint8][]string

var client *messagelib.MessageLib

func main() {
	var config implementation.ClientConfig
	err := util.ReadJSONConfig("config/client_config.json", &config)
	util.CheckErr(err, "config read")
	client = messagelib.NewMessageLib()
	messageChan, err := client.Start(config)
	util.CheckErr(err, "Error starting messagelib:")

	fmt.Println("Welcome to the Ephemeral Rocket 🚀")
	fmt.Println("---------------------")

	// MesMap = make(map[uint8][]string)
	reader := bufio.NewReader(os.Stdin)

	go CheckForMessages(messageChan)

	for {
		DisplayActions(0, 4)
		text, _ := reader.ReadString('\n')
		fmt.Println("input: ", text)
		HandleInput(text, reader)
	}

}

func HandleInput(input string, reader *bufio.Reader) {
	fmt.Println("input ->", input)
	switch input {
	case "0":
		fmt.Println("case 0")
		ViewMessages()
	case "1":
		fmt.Println("case 1")
		ViewClients()
	case "2":
		fmt.Println("case 2")
		SendMessage(reader)
	case "3":
		fmt.Println("case 3")
		fmt.Println("You thought you could quit, how foolish, you're stuck on this 🚀 forever muhahahaha")
	}
}

func ViewClients() {
	fmt.Println("Here are the available Clients To Launch a Rocket With")
	fmt.Println(client.ViewClients())
}

func SendMessage(reader *bufio.Reader) {
	fmt.Print("Compose you message \n🚀")
	text, _ := reader.ReadString('\n')
	fmt.Printf("your message ------ \n %s \n ------ \n", text)
	fmt.Println("This action can't be sent right now, our engines are currently undergoing maintence")
}

func ViewMessages() {
	fmt.Println("Here are your unseen messages")
	fmt.Println(client.ViewClients())
}

func CheckForMessages(mchan chan implementation.MessageStruct) {
	for {
		m := <-mchan
		fmt.Printf("\n New Messages From User %s 🚀\n", m.SourceId)
	}
}
func DisplayActions(start int, stop int) {
	fmt.Print("- - - Select an Action - - -\n\n")
	for n, val := range actions[start:stop] {
		fmt.Printf("%d: %s\n", n, val)
	}
}
