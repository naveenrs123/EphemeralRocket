package messagelib

import (
	"ephemeralrocket/implementation"
	"fmt"
	"net"
	"net/rpc"
	"time"
)

type MessageLib struct {
	clientId            string
	coordClient   		rpc.Client
	coordIPPort   		string
	localCoordIPPort  	string
	serverClient        rpc.Client
	primaryServerIPPort string
	primaryReady        bool
}

func NewMessageLib() *MessageLib {
	return &MessageLib{}
}

func (m *MessageLib) Start(config implementation.ClientConfig) error {
	laddr, lerr := net.ResolveTCPAddr("tcp", config.LocalCoordAddr)
	if lerr != nil {
		return fmt.Errorf("unable to resolve local coord ipport %s, with error %e", config.LocalCoordAddr, lerr)
	}
	raddr, rerr := net.ResolveTCPAddr("tcp", config.CoordAddr)
	if rerr != nil {
		return fmt.Errorf("unable to resolve remove coord ipport %s, with error %e", config.CoordAddr, rerr)
	}
	conn, cerr := net.DialTCP("tcp", laddr, raddr)
	if cerr != nil {
		return fmt.Errorf("unable to connect to coord node, with error %e", cerr)
	}
	m.clientId = config.ClientID
	m.coordClient = *rpc.NewClient(conn)
	m.coordIPPort = config.CoordAddr
	m.localCoordIPPort = config.LocalCoordAddr
	go m.GetPrimaryServer()
	return nil
}

func (m *MessageLib) ViewClients() []string {
	return make([]string, 0)
}

func (m *MessageLib) SendMessage(message implementation.MessageStruct) (implementation.MessageStruct, error) {
	for {
		if !m.primaryReady {
			continue
		}
		message.Timestamp = time.Now()
		
		err := m.serverClient.Call("ServerRPC.SendMessage", &message, &message)
		if err != nil {
			m.GetPrimaryServer()
			continue
		}
		break
	}
	return message, nil
}

func (m *MessageLib) RetrieveMessages(sourceClientId string) []implementation.MessageStruct {
	return make([]implementation.MessageStruct, 0)
}

func (m *MessageLib) Stop() {

}

// MessageLib is only an RPC Client. It will only call RPC methods on the coord and server.

// Required Internal Methods: BEGIN

// Decide based on how we split up the code. Everything apart from Start and Stop is blocking.

// Required Internal Methods: END

func (m *MessageLib) GetPrimaryServer() {
	var result implementation.PrimaryServerRes

	for {
		req := implementation.PrimaryServerReq{ClientId: m.clientId}
		err := m.coordClient.Call("CoordRPC.RetrievePrimaryServer", &req, &result)
		if err != nil {
			continue
		}

		if !result.ChainReady {
			fmt.Println("Server chain not ready yet")
			time.Sleep(time.Millisecond * 100)
			continue
		}
		break
	}
	m.primaryServerIPPort = result.PrimaryServerIPPort
	serverConn, err := getTCPConn(m.primaryServerIPPort)
	if err != nil {
		m.primaryReady = false
	} else {
		m.serverClient = *rpc.NewClient(serverConn)
		m.primaryReady = true
	}
}


func getTCPConn(remoteAddr string) (*net.TCPConn, error) {
	raddr, rerr := net.ResolveTCPAddr("tcp", remoteAddr)
	if rerr != nil {
		return nil, rerr
	}
	conn, connerr := net.DialTCP("tcp", nil, raddr)
	if connerr != nil {
		return nil, connerr
	} else {
		return conn, nil
	}

}


 
