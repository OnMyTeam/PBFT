// TODO: secure connection such as HTTPS, or manual implementation
// from Section 5.2.2 Key Exchanges on TOCS.
package network

import (
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"crypto/ecdsa"
	"encoding/json"
	//"fmt"
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"log"
	"time"
	//"sync"
)
type Server struct {
	url  string
	node *Node
}
 
func NewServer(nodeID string, nodeTable []*NodeInfo, viewID int64, decodePrivKey *ecdsa.PrivateKey) *Server {
	nodeIdx := int(-1)
	for idx, nodeInfo := range nodeTable {
		if nodeInfo.NodeID == nodeID {
			nodeIdx = idx
			break
		}
	}

	if nodeIdx == -1 {
		log.Printf("Node '%s' does not exist!\n", nodeID)
		return nil
	}

	node := NewNode(nodeTable[nodeIdx], nodeTable, viewID, decodePrivKey)
	server := &Server{
		url: nodeTable[nodeIdx].Url,
		node: node,
	}

	server.setRoute("/prepare")

	return server
}

func (server *Server) setRoute(path string) {
	hub := NewHub()
	handler := func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}
	http.HandleFunc(path, handler)

	go hub.run()
}

func (server *Server) Start() {
	log.Printf("%s Server will be started at %s...\n", server.node.MyInfo.NodeID, server.url)

	go server.DialOtherNodes()

	if err := http.ListenAndServe(server.url, nil); err != nil {
		log.Println(err)
		return
	}
}

func (server *Server) DialOtherNodes() {
	// Sleep until all nodes perform ListenAndServ().
	time.Sleep(time.Second * 3)

	var cPrepare = make(map[string]*websocket.Conn)

	for _, nodeInfo := range server.node.NodeTable {
		cPrepare[nodeInfo.NodeID] = server.setReceiveLoop("/prepare", nodeInfo)
	}
	time.Sleep(time.Second * 10)
	go server.sendDummyMsg()
	
	// Wait.
	select {}

	//defer c.Close()
}

func (server *Server) setReceiveLoop(path string, nodeInfo *NodeInfo) *websocket.Conn {
	u := url.URL{Scheme: "ws", Host: nodeInfo.Url, Path: path}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
		return nil
	}
	log.Printf("connecting to %s from %s for %s", nodeInfo.NodeID, server.node.MyInfo.NodeID,path)
	//log.Println("sRL local addr : ",c.LocalAddr(),"sRL remote addr : ",c.RemoteAddr())
	go server.receiveLoop(c, path, nodeInfo)

	return c
}
func (server *Server) receiveLoop(cc *websocket.Conn, path string, nodeInfo *NodeInfo) {
	c:=cc
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			u := url.URL{Scheme: "ws", Host: nodeInfo.Url, Path: path}
			c, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				log.Fatal("dial:", err)
				return 
			}
			_, message, err = c.ReadMessage()
			log.Printf("currunpted message size: %d\n", len(message))
			continue
		}
		var rawMsg consensus.SignatureMsg
		rawMsg, err, ok := deattachSignatureMsg(message, nodeInfo.PubKey)
		if err != nil {
			fmt.Println("[receiveLoop-error]", err)
		}
		if ok == false {
			fmt.Println("[receiveLoop-error] decoding error")
		}
		switch rawMsg.MsgType {
		case "/prepare":
			// ReqPrePareMsgs have RequestMsg and PrepareMsg
			var msg consensus.ReqPrePareMsgs
			_ = json.Unmarshal(rawMsg.MarshalledMsg, &msg)
			if msg.PrepareMsg.SequenceID == 0 {
				fmt.Println("[receiveLoop-error] seq 0 came in")
				continue
			}
			server.node.MsgEntrance <- &msg
		case "/vote":
			var msg consensus.VoteMsg
			_ = json.Unmarshal(rawMsg.MarshalledMsg, &msg)
			if msg.SequenceID == 0 {
				fmt.Println("[receiveLoop-error] seq 0 came in")
				continue
			}
			server.node.MsgEntrance <- &msg
		case "/collate":
			var msg consensus.CollateMsg
			_ = json.Unmarshal(rawMsg.MarshalledMsg, &msg)
			if msg.SequenceID == 0 {
				fmt.Println("[receiveLoop-error] seq 0 came in")
				continue
			}
			server.node.MsgEntrance <- &msg
		/*
		case "/checkpoint":
			var msg consensus.CheckPointMsg
			server.node.MsgEntrance <- &msg
		 */
		case "/viewchange":
			var msg consensus.ViewChangeMsg
			_ = json.Unmarshal(rawMsg.MarshalledMsg, &msg)
			server.node.ViewMsgEntrance <- &msg
		case "/newview":
			var msg consensus.NewViewMsg
			_ = json.Unmarshal(rawMsg.MarshalledMsg, &msg)
			server.node.ViewMsgEntrance <- &msg
		}
	}
}
func (server *Server) sendDummyMsg() {
	const sendPeriod time.Duration = 300

	ticker := time.NewTicker(time.Millisecond * sendPeriod)
	defer ticker.Stop()

	data := make([]byte, 1 << 20)
	for i := range data {
		data[i] = 'A'
	}
	data[len(data)-1]=0
	// data := consensus.randBytes(1024 * 1024)
	currentView := server.node.View.ID

	sequenceID := 0

	for  {
		select {
		case <-ticker.C:
			//if server.node.IsViewChanging {
			//	continue
			//}
			primaryNode := server.node.getPrimaryInfoByID(currentView)
			currentView++
			sequenceID += 1

			// if sequenceID == 10 {
			// 	time.Sleep(time.Second * 10)
			// }
			if primaryNode.NodeID != server.node.MyInfo.NodeID {
				continue
			}
			dummy := dummyMsg("Op1", "Client1", data, 
				server.node.View.ID,int64(sequenceID),
				server.node.MyInfo.NodeID)	

			// Broadcast the dummy message.
			errCh := make(chan error, 1)
			log.Printf("Broadcasting dummy message from %s, sequenceId: %d", server.node.MyInfo.Url, sequenceID)
			broadcast(errCh, server.node.MyInfo.Url, dummy, "/prepare", server.node.PrivKey)
			err := <-errCh
			if err != nil {
				log.Println(err)
			}
		}

	}
}
func broadcast(errCh chan<- error, url string, msg []byte, path string, privKey *ecdsa.PrivateKey) {
	sigMgsBytes := attachSignatureMsg(msg, privKey, path)
	url = "ws://" + url +"/prepare" // Fix using url.URL{}
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		errCh <- err
		return
	}
	err = c.WriteMessage(websocket.TextMessage, sigMgsBytes)
	if err != nil {
		errCh <- err
		return
	}
	defer c.Close()

	errCh <- nil
}
func attachSignatureMsg(msg []byte, privKey *ecdsa.PrivateKey, path string) []byte {
	var sigMgs consensus.SignatureMsg
	r,s,signature, err:=consensus.Sign(privKey, msg)
	if err == nil {
		sigMgs = consensus.SignatureMsg {
			Signature: signature,
			R: r,
			S: s,
			MarshalledMsg: msg,
			MsgType: path,
		}
	}
	sigMgsBytes, _ := json.Marshal(&sigMgs)
	return sigMgsBytes
}
func deattachSignatureMsg(msg []byte, pubkey *ecdsa.PublicKey)(consensus.SignatureMsg,
		error, bool){
	var sigMgs consensus.SignatureMsg
	err := json.Unmarshal(msg, &sigMgs)
	ok := false
	if err != nil {
		return sigMgs, err, false
	}
	ok = consensus.Verify(pubkey, sigMgs.R, sigMgs.S, sigMgs.MarshalledMsg)
	return sigMgs, nil, ok
}
func dummyMsg(operation string, clientID string, data []byte, 
		viewID int64, sID int64, nodeID string) []byte {
	var RequestMsg consensus.RequestMsg
	RequestMsg.Timestamp = time.Now().UnixNano()
	RequestMsg.Operation = operation
	RequestMsg.ClientID = clientID
	RequestMsg.Data = string(data)
	RequestMsg.SequenceID = sID
	
	digest, err := consensus.Digest(RequestMsg)

	var PrepareMsg consensus.PrepareMsg
	PrepareMsg.ViewID = viewID
	PrepareMsg.SequenceID = sID
	PrepareMsg.Digest = digest
	PrepareMsg.EpochID = 0
	PrepareMsg.NodeID = nodeID

	var ReqPrePareMsgs consensus.ReqPrePareMsgs
	ReqPrePareMsgs.RequestMsg = &RequestMsg
	ReqPrePareMsgs.PrepareMsg = &PrepareMsg

	jsonMsg, err := json.Marshal(ReqPrePareMsgs)
	if err != nil {
		log.Println(err)
		return nil
	}

	return []byte(jsonMsg)
}
