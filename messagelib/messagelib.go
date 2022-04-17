package messagelib

import (
	"ephemeralrocket/implementation"
	"ephemeralrocket/util"
	"fmt"
	//"io/ioutil"
	"net"
	"net/rpc"

	//"os"
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
	knownClients        []string
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
	err := conn.SetLinger(0)
	if err != nil {
		fmt.Println(err)
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
	var req interface{}
	var res implementation.RetrieveClientsRes
	err := m.coordClient.Call("CoordRPC.RetrieveClients", &req, &res)
	if err != nil {
		// this error should never occur, coord should never fail
		util.CheckErr(err, "Error retrieving clients")
	}
	m.knownClients = res.ClientIds
	return res.ClientIds
}

/**
Sends a message to a destination client. This call is blocking until the RPC call succeeds.
Echos the message sent back, with a timestamp
**/
func (m *MessageLib) SendMessage(message implementation.MessageStruct) (implementation.MessageStruct, error) {
	if !m.isClientValid(message.DestinationId, false) {
		return implementation.MessageStruct{}, fmt.Errorf("invalid client")
	}
	for {
		if !m.primaryReady {
			continue
		}
		message.Timestamp = time.Now()

		if !util.IsConnectionAlive(m.primaryServerIPPort) {
			m.serverClient.Close()
			m.GetPrimaryServer()
			continue
		}
		err := m.serverClient.Call("ServerRPC.ReceiveSenderMessage", &message, &message)
		if err != nil {
			m.serverClient.Close()
			m.GetPrimaryServer()
			continue
		}
		break
	}
	return message, nil
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
		time.Sleep(time.Second * 5)
		select {
		case <-m.quitChan:
			break outer

		default:
			var res implementation.RetrieveMessageRes
			if !m.primaryReady {
				continue
			}
			req := implementation.RetrieveMessageReq{ClientId: m.clientId}
			if !util.IsConnectionAlive(m.primaryServerIPPort) {
				m.serverClient.Close()
				m.GetPrimaryServer()
				continue
			}
			err := m.serverClient.Call("ServerRPC.RetrieveMessages", &req, &res)
			if err != nil {
				m.serverClient.Close()
				m.GetPrimaryServer()
				continue
			}
			for i := 0; i < len(res.Messages); i++ {
				m.messageChan <- res.Messages[i]
			}
		}
	}

}

func (m *MessageLib) isClientValid(clientId string, checkCoord bool) bool {
	if checkCoord { // call RPC call in coord to retrieve current list of clients, then cache it
		var req interface{}
		var res implementation.RetrieveClientsRes
		err := m.coordClient.Call("CoordRPC.RetrieveClients", &req, &res)
		if err != nil {
			// this should never happen because the coord should never fail
			util.CheckErr(err, "Error retrieving clients")
		}
		m.knownClients = res.ClientIds
	}
	if util.FindElement(m.knownClients, clientId) {
		return true
	} else {
		if !checkCoord { // if coord wasn't checked, call method again and check coord
			return m.isClientValid(clientId, true)
		} else {
			return false
		}
	}
}

func (m *MessageLib) GetPrimaryServer() {
	var result implementation.PrimaryServerRes

	for {
		req := implementation.PrimaryServerReq{ClientId: m.clientId}
		err := m.coordClient.Call("CoordRPC.RetrievePrimaryServer", &req, &result)
		if err != nil {
			// this should never happen because the coord should never fail
			util.CheckErr(err, "Error retrieving primary server")
			continue
		}

		if !result.ChainReady {
			// chain not ready, wait and try again
			time.Sleep(time.Millisecond * 100)
			m.primaryReady = false
			continue
		}
		m.primaryServerIPPort = result.PrimaryServerIPPort
		serverConn, err := util.GetTCPConn(m.primaryServerIPPort)
		if err != nil {
			// there is an error connecting to the primary returned by the coord, try again
			m.primaryReady = false
			time.Sleep(time.Millisecond * 10)
		} else {
			m.serverClient = *rpc.NewClient(serverConn)
			m.primaryReady = true
			break
		}
	}
}
