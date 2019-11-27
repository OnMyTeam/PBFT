// TODO: secure connection such as HTTPS, or manual implementation
// from Section 5.2.2 Key Exchanges on TOCS.
package network

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	//"fmt"
	//"sync/atomic"
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"log"
	"time"
	//"sync"
)
const sendPeriod time.Duration = 350
type Server struct {
	url  string
	node *Node
}
 
func NewServer(nodeID string, nodeTable []*NodeInfo, seedNodeTables [20][]*NodeInfo,
			viewID int64, decodePrivKey *ecdsa.PrivateKey) *Server {
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

	node := NewNode(nodeTable[nodeIdx], nodeTable, seedNodeTables, viewID, decodePrivKey)
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
	time.Sleep(time.Second * 3)
	server.sendGenesisMsgIfPrimary()
	
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
		/////////////////////////////////////////////////////////////////////////////////////////////
		time.Sleep(time.Millisecond * 100)		
		switch rawMsg.MsgType {
		case "/prepare":
			// ReqPrePareMsgs have RequestMsg and PrepareMsg
			var msg consensus.ReqPrePareMsgs
			_ = json.Unmarshal(rawMsg.MarshalledMsg, &msg)
			if msg.PrepareMsg.SequenceID == 0 {
				fmt.Println("[receiveLoop-error] seq 0 came in")
				continue
			}
			// fmt.Println("[EndPrepare] to:",server.node.MyInfo.NodeID,"from:",msg.PrepareMsg.NodeID, "/",time.Now().UnixNano())
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

func (server *Server) sendGenesisMsgIfPrimary() {
	var sequenceID int64 = 1
	var seed int = -1

	data := make([]byte, 1 << 20)
	for i := range data {
		data[i] = 'A'
	}
	data[len(data)-1]=0

	server.node.updateViewID(sequenceID-1)
	server.node.updateEpochID(sequenceID-1)
	primaryNode := server.node.getPrimaryInfoByID(server.node.View.ID)

	fmt.Printf("server.node.MyInfo.NodeID: %s\n", server.node.MyInfo.NodeID)
	fmt.Printf("primaryNode.NodeID: %s\n", primaryNode.NodeID)
		
	if primaryNode.NodeID != server.node.MyInfo.NodeID {
		return
	}
	
	prepareMsg := PrepareMsgMaking("Op1", "Client1", data, 
		server.node.View.ID,int64(sequenceID),
		server.node.MyInfo.NodeID, int(seed), server.node.EpochID)

	log.Printf("Broadcasting dummy message from %s, sequenceId: %d, epochId: %d, viewId: %d",
		server.node.MyInfo.NodeID, sequenceID, server.node.EpochID, server.node.View.ID)

	fmt.Println("[StartPrepare]", "seqID",sequenceID, time.Now().UnixNano())
	server.node.Broadcast(prepareMsg, "/prepare")
	// dummy := dummyMsg("Op1", "Client1", data, 
	// 	server.node.View.ID,int64(sequenceID),
	// 	server.node.MyInfo.NodeID, seed)
	// //currentView := server.node.View.ID

	// sequenceID := int64(0)
	// committee_num := int64(10)

	// for  {
	// 	select {
	// 	case <-ticker.C:
	// 		if server.node.IsViewChanging {
	// 			continue
	// 		}

	// 		sequenceID++

	// 		server.node.updateView(sequenceID-1)
	// 		server.node.updateEpochID(sequenceID-1)
	// 		primaryNode := server.node.getPrimaryInfoByID(server.node.View.ID)
			
			

	// 		fmt.Printf("server.node.MyInfo.NodeID: %s\n", server.node.MyInfo.NodeID)
	// 		fmt.Printf("primaryNode.NodeID: %s\n", primaryNode.NodeID)
			
			
	// 		if primaryNode.NodeID != server.node.MyInfo.NodeID {
	// 			continue
	// 		}

	// 		log.Printf("server.node.View.ID: %d", server.node.View.ID)
	// 		dummy := dummyMsg("Op1", "Client1", data, 
	// 			server.node.View.ID,int64(sequenceID),
	// 			server.node.MyInfo.NodeID, server.node.EpochID)	

	// 		// Broadcast the dummy message.
	// 		errCh := make(chan error, 1)
	// 		log.Printf("Broadcasting dummy message from %s, sequenceId: %d, viewid: %d, epoch: %d", server.node.MyInfo.Url, sequenceID, server.node.View.ID, server.node.EpochID)
	// 		fmt.Println("[StartPrepare] sequenceId:", sequenceID, time.Now().UnixNano())
	// 		broadcast(errCh, server.node.MyInfo.Url, dummy, "/prepare", server.node.PrivKey)
			


	// 		err := <-errCh
	// 		if err != nil {
	// 			log.Println(err)
	// 		}
	// 	case viewchangechannel := <- server.node.ViewChangeChan:
			
		
	// 		if  viewchangechannel.VCSCheck {
				
	// 			server.node.StableCheckPoint = viewchangechannel.Min_S
	// 			sequenceID = server.node.StableCheckPoint+1

	// 			server.node.updateView(server.node.StableCheckPoint)
	// 			server.node.updateEpochID(server.node.StableCheckPoint)
				
	// 			fmt.Println("server.node.NextCandidateIdx: ", server.node.NextCandidateIdx)
	// 			primaryNode := server.node.NodeTable[server.node.NextCandidateIdx]

				
	// 			fmt.Println("[VIEWCHANGE_DONE] ",",",sequenceID,",",time.Since(server.node.VCStates[server.node.NextCandidateIdx].GetReceiveViewchangeTime()))

	// 			atomic.AddInt64(&server.node.NextCandidateIdx, 1)

	// 			if sequenceID % committee_num == 0 {
	// 				server.node.VCStates = make(map[int64]*consensus.VCState)
	// 				server.node.NextCandidateIdx = committee_num
	// 				//primaryNode = server.node.NodeTable[server.node.NextCandidateIdx]
	// 			}

	// 			go server.node.startTransitionWithDeadline(nil)
	// 			server.node.IsViewChanging = false

	// 			if primaryNode.NodeID != server.node.MyInfo.NodeID {
	// 				continue
	// 			}

	// 			//log.Printf("--------------------------------view-change--------------------------------\n")
	// 			dummy := dummyMsg("Op1", "Client1", data, 
	// 				server.node.View.ID,int64(sequenceID),
	// 				server.node.MyInfo.NodeID, server.node.EpochID)
				

	// 			// Broadcast the dummy message.
	// 			errCh := make(chan error, 1)
				

	// 			log.Printf("Broadcasting dummy message from %s, sequenceId: %d, viewid: %d, epoch: %d ", server.node.MyInfo.Url, sequenceID, server.node.View.ID, server.node.EpochID)
	// 			broadcast(errCh, server.node.MyInfo.Url, dummy, "/prepare", server.node.PrivKey)
			
	// 			err := <-errCh
	// 			if err != nil {
	// 				log.Println(err)
	// 			}
	// 		}
	// 	}

	// errCh := make(chan error, 1)
	
	// log.Printf("Broadcasting dummy message from %s, sequenceId: %d",
	// 	server.node.MyInfo.NodeID, sequenceID)
	// fmt.Println("[StartPrepare]", "seqID",sequenceID, time.Now().UnixNano())
	// broadcast(errCh, server.node.MyInfo.Url, dummy, "/prepare", server.node.PrivKey)
	// err := <-errCh
	// if err != nil {
	// 	log.Println(err)
	// }
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
		//log.Println("dettachSignature error ", err)
		return sigMgs, err, false
	}
	ok = consensus.Verify(pubkey, sigMgs.R, sigMgs.S, sigMgs.MarshalledMsg)
	return sigMgs, nil, ok
}
func PrepareMsgMaking(operation string, clientID string, data []byte, 
	viewID int64, sID int64, nodeID string, Seed int, epochID int64) *consensus.ReqPrePareMsgs {
	var RequestMsg consensus.RequestMsg
	RequestMsg.Timestamp = time.Now().UnixNano()
	RequestMsg.Operation = operation
	RequestMsg.ClientID = clientID
	RequestMsg.Data = string(data)
	RequestMsg.SequenceID = sID
	
	digest, err := consensus.Digest(RequestMsg)

	if err != nil {
		fmt.Println(err)
	}

	var PrepareMsg consensus.PrepareMsg
	PrepareMsg.ViewID = viewID
	PrepareMsg.SequenceID = sID
	PrepareMsg.Digest = digest
	PrepareMsg.EpochID = epochID
	PrepareMsg.NodeID = nodeID
	PrepareMsg.Seed= Seed

	var ReqPrePareMsgs consensus.ReqPrePareMsgs
	ReqPrePareMsgs.RequestMsg = &RequestMsg
	ReqPrePareMsgs.PrepareMsg = &PrepareMsg

	return &ReqPrePareMsgs
}
// func dummyMsg(operation string, clientID string, data []byte, 
// 		viewID int64, sID int64, nodeID string, Seed int) []byte {
// 	var RequestMsg consensus.RequestMsg
// 	RequestMsg.Timestamp = time.Now().UnixNano()
// 	RequestMsg.Operation = operation
// 	RequestMsg.ClientID = clientID
// 	RequestMsg.Data = string(data)
// 	RequestMsg.SequenceID = sID
	
// 	digest, err := consensus.Digest(RequestMsg)

// 	var PrepareMsg consensus.PrepareMsg
// 	PrepareMsg.ViewID = viewID
// 	PrepareMsg.SequenceID = sID
// 	PrepareMsg.Digest = digest
// 	PrepareMsg.EpochID = 0
// 	PrepareMsg.NodeID = nodeID
// 	PrepareMsg.Seed= Seed

// 	var ReqPrePareMsgs consensus.ReqPrePareMsgs
// 	ReqPrePareMsgs.RequestMsg = &RequestMsg
// 	ReqPrePareMsgs.PrepareMsg = &PrepareMsg

// 	jsonMsg, err := json.Marshal(ReqPrePareMsgs)
// 	if err != nil {
// 		log.Println(err)
// 		return nil
// 	}

// 	return []byte(jsonMsg)
// }
