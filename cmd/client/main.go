package main

import (
	"bufio"
	"ephemeralrocket/implementation"
	"ephemeralrocket/messagelib"
	"ephemeralrocket/util"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

var actions []string = []string{"View Messages",
	// "Start a new converstaion",
	"Send Message", "quit"}

var MesMap map[string][]implementation.MessageStruct

var client *messagelib.MessageLib

func main() {
	var config implementation.ClientConfig
	fmt.Println(os.Args[1])
	err := util.ReadJSONConfig(fmt.Sprintf("config/client_config%s.json", os.Args[1]), &config)
	util.CheckErr(err, "config read")
	client = messagelib.NewMessageLib()
	messageChan, err := client.Start(config)
	util.CheckErr(err, "Error starting messagelib:")

	fmt.Println("Welcome to the super Ephemeral Rocket 🚀")
	fmt.Println("---------------------")

	MesMap = make(map[string][]implementation.MessageStruct)
	reader := bufio.NewReader(os.Stdin)

	LoadMessages(config)

	go CheckForMessages(messageChan)

	for {
		DisplayActions(0, 3)
		text, _ := reader.ReadString('\n')
		if HandleInput(text, reader, config) {
			return
		}

	}

}

func HandleInput(input string, reader *bufio.Reader, config implementation.ClientConfig) bool {
	switch strings.TrimSpace(input) {
	case "0":
		fmt.Println("case 0")
		ViewMessages(reader)
	// case "1":
	// 	fmt.Println("case 1")
	// 	ViewClients()
	case "1":
		fmt.Println("case 2")
		SendMessage(reader, config)
	case "2":
		fmt.Println("case 3")
		client.Stop()
		fmt.Println("Goodbye!")
		return true
	}

	return false
}

func ViewClients() {
	fmt.Println("Here are the available Clients To message")
	fmt.Println(client.ViewClients())
}

func SendMessage(reader *bufio.Reader, config implementation.ClientConfig) {
	ViewClients()
	fmt.Println("which client would you like to message")
	dest, _ := reader.ReadString('\n')
	fmt.Print("Compose you message \n🚀")
	mess, _ := reader.ReadString('\n')
	message := implementation.MessageStruct{config.ClientID, dest, mess, time.Now()}
	res, err := client.SendMessage(message)
	util.CheckErr(err, "Error Sending Message")
	MesMap[dest] = append(MesMap[dest], res)

}

func ViewMessages(reader *bufio.Reader) {
	fmt.Println("Which user's Messages would you like to see?")
	for k, _ := range MesMap {
		fmt.Println(k)
	}
	user, _ := reader.ReadString('\n')
	if _, ok := MesMap[user]; ok {
		for _, v := range MesMap[user] {
			fmt.Printf("%s: %s\n", v.SourceId, v.Data)
		}
	} else {
		fmt.Println("Invalid User Id")
	}
}

func CheckForMessages(mchan chan implementation.MessageStruct) {
	for {
		m := <-mchan
		fmt.Printf("\n New Messages From User %s 🚀\n", m.SourceId)
		MesMap[m.SourceId] = append(MesMap[m.SourceId], m)
		OrderMessages(m.SourceId)
	}
}

func LoadMessages(config implementation.ClientConfig) {
	res := client.RetrieveMessages(config.ClientID)
	for _, v := range res {
		MesMap[v.SourceId] = append(MesMap[v.SourceId], v)
	}
	OrderMessages("")
}

func OrderMessages(source string) {
	if source == "" {
		for key, _ := range MesMap {
			sort.SliceStable(MesMap[key], func(i, j int) bool {
				return MesMap[key][i].Timestamp.Before(MesMap[key][j].Timestamp)
			})
		}
	} else {
		sort.SliceStable(MesMap[source], func(i, j int) bool {
			return MesMap[source][i].Timestamp.Before(MesMap[source][j].Timestamp)
		})
	}
}

func DisplayActions(start int, stop int) {
	fmt.Print("- - - Select an Action - - -\n\n")
	for n, val := range actions[start:stop] {
		fmt.Printf("%d: %s\n", n, val)
	}
	fmt.Print("\n")
}
