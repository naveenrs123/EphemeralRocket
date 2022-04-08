package main

import (
	"bufio"
	"ephemeralrocket/implementation"
	"ephemeralrocket/messagelib"
	"ephemeralrocket/util"
	"fmt"
	"os"
	"sort"
	"strconv"
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
	fmt.Println("⭐🌝⭐🌚⭐🌝⭐🌚⭐🌝⭐🌚⭐🌝⭐🌚⭐🌝⭐🌚⭐🌝⭐🌚⭐🌝⭐")

	MesMap = make(map[string][]implementation.MessageStruct)
	reader := bufio.NewReader(os.Stdin)

	go CheckForMessages(messageChan)

	for {
		DisplayActions(0, 3)
		text := GetInput(reader)
		if HandleInput(text, reader, config) {
			return
		}

	}

}

func HandleInput(input string, reader *bufio.Reader, config implementation.ClientConfig) bool {
	switch input {
	case "0":
		ViewMessages(reader)
	case "1":
		SendMessage(reader, config)
	case "2":
		client.Stop()
		fmt.Println("Goodbye!")
		return true
	}

	return false
}

func ViewClients() []string {
	fmt.Println("Here are the available Clients To message")
	return client.ViewClients()
}

func SendMessage(reader *bufio.Reader, config implementation.ClientConfig) {
	for {
		clients := ViewClients()
		PrintIndexAndValue(clients)
		fmt.Println("Enter the number of the client you'd like to message")
		index, _ := strconv.Atoi(GetInput(reader))
		if index-1 > len(clients)-1 {
			fmt.Println("Invalid client id, please try again")
			continue
		}
		dest := clients[index-1]
		fmt.Print("Compose you message \n")
		mess := GetInput(reader)
		message := implementation.MessageStruct{SourceId: config.ClientID, DestinationId: dest, Data: mess}
		res, err := client.SendMessage(message)
		if err != nil {
			fmt.Println("Invalid user id, please try again")
		} else {
			fmt.Printf("Message sent at %s!", GetDateTimeString(res.Timestamp))
			MesMap[dest] = append(MesMap[dest], res)
			break
		}
	}
}

func ViewMessages(reader *bufio.Reader) {
	if len(MesMap) == 0 {
		fmt.Println("No messages")
	} else {
		for {
			fmt.Println("Enter the number of the client whose messages you'd like to see?")
			keys := make([]string, len(MesMap))
			i := 0
			for k := range MesMap {
				keys[i] = k
				i++
			}
			PrintIndexAndValue(keys)
			index, _ := strconv.Atoi(GetInput(reader))
			if index-1 > len(keys)-1 {
				fmt.Println("Invalid index, please try again.")
				continue
			}
			user := keys[index-1]
			if _, ok := MesMap[user]; ok {
				for _, v := range MesMap[user] {
					fmt.Printf("%s: %s        delivered: %s\n", v.SourceId, v.Data, GetDateTimeString(v.Timestamp))
				}
				break
			} else {
				fmt.Println("Invalid User Id")
				continue
			}
		}
	}
}

func CheckForMessages(mchan chan implementation.MessageStruct) {
	for {
		m := <-mchan
		fmt.Printf("\n New Messages From User %s 🚀\n", m.SourceId)
		MesMap[m.SourceId] = append(MesMap[m.SourceId], m)
		OrderMessages(m.SourceId)
		fmt.Print("🚀 ->")
	}
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
	fmt.Print("\n- - - Select an Action - - -\n\n")
	for n, val := range actions[start:stop] {
		fmt.Printf("%d: %s\n", n, val)
	}
	fmt.Print("\n")
}

func GetInput(reader *bufio.Reader) string {
	fmt.Print("🚀 ->")
	in, _ := reader.ReadString('\n')
	return strings.TrimSpace(in)
}

func PrintIndexAndValue(arr []string) {
	for i, v := range arr {
		fmt.Printf("%d: %s\n", i+1, v)
	}
}

func GetDateTimeString(time time.Time) string {
	return fmt.Sprintf("%d-%d-%d %d:%d", time.Month(), time.Day(), time.Year(), time.Hour(), time.Minute())
}
