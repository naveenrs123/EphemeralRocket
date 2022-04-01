package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/rpc"
	"os"
	"strings"
)

func ReadJSONConfig(filename string, config interface{}) error {
	configData, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	err = json.Unmarshal(configData, config)
	if err != nil {
		return err
	}
	return nil
}

func CheckErr(err error, errfmsg string, fargs ...interface{}) {
	if err != nil {
		fmt.Fprintf(os.Stderr, errfmsg, fargs...)
		fmt.Printf("ERROR: %v", err)
		os.Exit(1)
	}
}

// Serve an RPC connection that indefinitely listens for new calls
func CreateRPCConn(listener *net.TCPListener) {
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			continue // bad call, just try again
		}
		go rpc.ServeConn(conn)
	}
}

// Find a random available port for new connections from the given IP address or IP:Port combination
func GetRandomPort(addr string) string {
	ip := ExtractIP(addr)
	port := "0"

	for {
		ln, err := net.Listen("tcp", ip+":"+port)
		port = ExtractPort(ln.Addr().String())
		ln.Close()
		if err == nil {
			break
		}
	}

	return port
}

// Extract the IP address from an IP:Port combination or return the original
// string if extraction was not possible.
func ExtractIP(addr string) string {
	splitAddr := strings.Split(addr, ":")
	return splitAddr[0]
}

// Extract the Port from an IP:Port combination or return :0 if extraction was not possible.
func ExtractPort(addr string) string {
	splitAddr := strings.Split(addr, ":")
	if len(splitAddr) == 1 {
		return ":0"
	}
	return splitAddr[1]
}

// removes a string from an array
func RemoveElement(arr []string, elem string) []string {
	for i, v := range arr {
		if v == elem {
			return append(arr[:i], arr[i+1:]...)
		}
	}
	return arr
}
