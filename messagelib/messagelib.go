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
	coordClient         rpc.Client
	coordIPPort         string
	localCoordIPPort    string
	serverClient        rpc.Client
	primaryServerIPPort string
	primaryReady        bool
	messageChan         chan implementation.MessageStruct
	quitChan            chan bool
}

func NewMessageLib() *MessageLib {
	return &MessageLib{}
}

/**
Starts the message lib, connects to the coord, requests a primary server, and polls for messages. Returns a channel
where messages can be passed to the client. Returns error if message lib is unable to connect to the coord.
**/
func (m *MessageLib) Start(config implementation.ClientConfig) (chan implementation.MessageStruct, error) {
	laddr, lerr := net.ResolveTCPAddr("tcp", config.LocalCoordAddr)
	if lerr != nil {
		return nil, fmt.Errorf("unable to resolve local coord ipport %s, with error %e", config.LocalCoordAddr, lerr)
	}
	raddr, rerr := net.ResolveTCPAddr("tcp", config.CoordAddr)
	if rerr != nil {
		return nil, fmt.Errorf("unable to resolve remove coord ipport %s, with error %e", config.CoordAddr, rerr)
	}
	conn, cerr := net.DialTCP("tcp", laddr, raddr)
	if cerr != nil {
		return nil, fmt.Errorf("unable to connect to coord node, with error %e", cerr)
	}
	m.clientId = config.ClientID
	m.coordClient = *rpc.NewClient(conn)
	m.coordIPPort = config.CoordAddr
	m.localCoordIPPort = config.LocalCoordAddr
	go m.GetPrimaryServer()
	m.messageChan = make(chan implementation.MessageStruct)
	m.quitChan = make(chan bool)
	go m.PollForMessages()
	return m.messageChan, nil
}

/**
Requests a list of clients available to be messaged from the coord. This should always succeed and shouldn't return an error,
since we assume the coord never fails.
**/
func (m *MessageLib) ViewClients() []string {
	for {
		var req interface{}
		var res implementation.RetrieveClientsRes
		err := m.coordClient.Call("CoordRPC.RetrieveClients", &req, &res)
		if err != nil {
			fmt.Printf("Error encountered retrieving clients")
			fmt.Println(err)
			continue
		}
		return res.ClientIds
	}
}

/**
Sends a message to a destination client. This call is blocking until the RPC call succeeds.
Echos the message sent back, with a timestamp
**/
func (m *MessageLib) SendMessage(message implementation.MessageStruct) implementation.MessageStruct {
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
	return message
}

/**
Retrieve messages from the primary server. If source client id is not nil, it only returns messages from that source client.
This call is blocking until the RPC call succeeds
**/
func (m *MessageLib) RetrieveMessages(sourceClientId string) []implementation.MessageStruct {
	var res implementation.RetrieveMessageRes
	for {
		if !m.primaryReady {
			continue
		}
		req := implementation.RetrieveMessageReq{ClientId: m.clientId, SourceClientId: sourceClientId}
		err := m.serverClient.Call("ServerRPC.RetrieveMessages", &req, &res)
		if err != nil {
			m.GetPrimaryServer()
			continue
		}
		return res.Messages
	}
}

/**
Stops message lib, closing all connections
**/
func (m *MessageLib) Stop() {
	m.coordClient.Close()
	m.serverClient.Close()
	m.quitChan <- true
}

func (m *MessageLib) PollForMessages() {
outer:
	for {
		time.Sleep(time.Second * 1)
		select {
		case <-m.quitChan:
			break outer

		default:
			var res implementation.RetrieveMessageRes
			if !m.primaryReady {
				continue
			}
			req := implementation.RetrieveMessageReq{ClientId: m.clientId}
			err := m.serverClient.Call("ServerRPC.RetrieveMessages", &req, &res)
			if err != nil {
				m.GetPrimaryServer()
				continue
			}
			for i := 0; i < len(res.Messages); i++ {
				m.messageChan <- res.Messages[i]
			}
		}
	}

}

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
