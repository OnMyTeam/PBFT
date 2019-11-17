package network

import (
	"encoding/json"
	"fmt"
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"time"
	// "context"
	"crypto/ecdsa"
	//"log"
	"sync"
	"sync/atomic"
)

type Node struct {
	MyInfo          *NodeInfo
	PrivKey         *ecdsa.PrivateKey
	NodeTable       []*NodeInfo
	SeedNodeTables	[20][]*NodeInfo
	View            *View

	States          map[int64]consensus.PBFT // key: sequenceID, value: state
	VCStates		map[int64]*consensus.VCState
	CommittedMsgs   map[int64]*consensus.PrepareMsg // kinda block.

	Committed		[1000]int64
	Prepared 		[1000]int64

	//ViewChangeState *consensus.ViewChangeState
	TotalConsensus  int64 // atomic. number of consensus started so far.
	IsViewChanging  bool

	// Channels
	MsgEntrance   chan interface{}
	MsgDelivery   chan interface{}
	MsgExecution  chan *consensus.PrepareMsg
	MsgOutbound   chan *MsgOut
	MsgError      chan []error
	ViewMsgEntrance chan interface{}

	// Mutexes for preventing from concurrent access
	StatesMutex sync.RWMutex
	VCStatesMutex sync.RWMutex
	CommittedMutex sync.RWMutex
	PreparedMutex sync.RWMutex

	// Saved checkpoint messages on this node
	// key: sequenceID, value: map(key: nodeID, value: checkpointMsg)
	CheckPointMutex     sync.RWMutex
	CheckPointMsgsLog   map[int64]map[string]*consensus.CheckPointMsg

	// The stable checkpoint that 2f + 1 nodes agreed
	StableCheckPoint    int64
}

type NodeInfo struct {
	NodeID string `json:"nodeID"`
	Url    string `json:"url"`
	PubKey *ecdsa.PublicKey
}

type View struct {
	ID      int64
	Primary *NodeInfo
}

// Outbound message
type MsgOut struct {
	IP   string
	Msg  []byte
	Path string
}

// Deadline for the consensus state.
// const ConsensusDeadline = time.Millisecond * 5000

// Cooling time to escape frequent error, or message sending retry.
const CoolingTime = time.Millisecond * 2

// Number of error messages to start cooling.
const CoolingTotalErrMsg = 30

// Number of outbound connection for a node.
const MaxOutboundConnection = 1000

func NewNode(myInfo *NodeInfo, nodeTable []*NodeInfo, seedNodeTables [20][]*NodeInfo,
			viewID int64, decodePrivKey *ecdsa.PrivateKey) *Node {
	node := &Node{
		MyInfo:    myInfo,
		PrivKey: decodePrivKey,
		NodeTable: nodeTable,
		SeedNodeTables: seedNodeTables,
		View:      &View{},
		IsViewChanging: false,

		// Consensus-related struct
		States:          make(map[int64]consensus.PBFT),
		VCStates: 		 make(map[int64]*consensus.VCState),
		
		CheckPointMsgsLog: make(map[int64]map[string]*consensus.CheckPointMsg),
		StableCheckPoint:  0,

		CommittedMsgs:   make(map[int64]*consensus.PrepareMsg),

		// Channels
		MsgEntrance: make(chan interface{}, len(nodeTable) * len(nodeTable)),
		MsgDelivery: make(chan interface{}, len(nodeTable) * len(nodeTable)), // TODO: enough?
		MsgExecution: make(chan *consensus.PrepareMsg, len(nodeTable)),
		MsgOutbound: make(chan *MsgOut, len(nodeTable)),
		MsgError: make(chan []error, len(nodeTable)),
		ViewMsgEntrance: make(chan interface{}, len(nodeTable)*3),
	}

	atomic.StoreInt64(&node.TotalConsensus, 0)
	node.updateView(viewID)

	// Start message dispatcher
	for i:=0; i < len(nodeTable); i++ {
		go node.dispatchMsg()
	}

	for i := 0; i < len(nodeTable); i++ {
		// Start message resolver
		go node.resolveMsg()
	}

	// Start message executor
	go node.executeMsg()

	// Start outbound message sender
	go node.sendMsg()

	// Start message error logger
	go node.logErrorMsg()

	return node
}

// Broadcast marshalled message.
func (node *Node) Broadcast(msg interface{}, path string) {
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		node.MsgError <- []error{err}
		return
	}
	node.MsgOutbound <- &MsgOut{IP: node.MyInfo.Url, Msg: jsonMsg, Path: path}
}
func (node *Node) startTransitionWithDeadline(seqID int64, state consensus.PBFT) {
	// time.Sleep(time.Millisecond*sendPeriod)
	// Set deadline based on the given timestamp.
	// var timeStamp int64 = time.Now().UnixNano()
	// sec := timeStamp / int64(time.Second)
	// nsec := timeStamp % int64(time.Second)
	// d := time.Unix(sec, nsec).Add(ConsensusDeadline)
	// ctx, cancel := context.WithDeadline(context.Background(), d)
	// defer cancel()
	// Check the time is skewed.
	//timeDiff := time.Until(d).Nanoseconds()

	// The node can receive messages for any consensus stage,
	// regardless of the current stage for the state.

	var sigma	[4]time.Duration
	sigma[consensus.NumOfPhase("Prepare")] = 1500
	sigma[consensus.NumOfPhase("Vote")] = 5000
	sigma[consensus.NumOfPhase("Collate")] = 5000
	sigma[consensus.NumOfPhase("ViewChange")] = 10000

	var timerArr			[4]*time.Timer
	var cancelCh			[4]chan struct {}

	MsgCh := state.GetMsgReceiveChannel()
	TimerStartCh := state.GetTimerStartReceiveChannel()
	TimerStopCh := state.GetTimerStopReceiveChannel()
	ExitCh := state.GetMsgExitReceiveChannel()
	go func() {
		for {
			select {
			case msgState := <-MsgCh:
				switch msg := msgState.(type) {
				case *consensus.ReqPrePareMsgs:
					node.GetPrepare(state, msg)
				case *consensus.VoteMsg:
					node.GetVote(state, msg)
				case *consensus.CollateMsg:
					node.GetCollate(state, msg)
				}
			case phaseName := <-TimerStartCh:
				phase:=consensus.NumOfPhase(phaseName)
				timerArr[phase] = time.NewTimer(time.Millisecond*sigma[phase])
				cancelCh[phase] = make(chan struct {})
				// fmt.Printf("[Seq %d Thread] Start %s Timer\n", seqID, phaseName)
				go func(phase int64, phaseName string) {
					select {
					case <-timerArr[phase].C: //when timer is done
					switch phaseName{
					case "Prepare":
						reqPrepareMsg := &consensus.ReqPrePareMsgs{
							RequestMsg: nil,
							PrepareMsg: &consensus.PrepareMsg{
								NodeID: node.MyInfo.NodeID,
								SequenceID: seqID,
								Seed: -1,
							},
						}
						node.GetPrepare(state, reqPrepareMsg)
					}
					case <-cancelCh[phase]: //when timer stop
					}
				}(phase, phaseName)
			case phaseName := <-TimerStopCh:
				phase := consensus.NumOfPhase(phaseName)
				if timerArr[phase] != nil{
					timerArr[phase].Stop()
				}
				cancelCh[phase] <- struct {}{}
			case <-ExitCh:
				fmt.Printf("[Terminate Thread] seqId %d finished!!!\n", state.GetSequenceID())
				node.StatesMutex.Lock()
				node.States[seqID] = nil
				node.StatesMutex.Unlock()
				return
				//case <-ctx.Done():
				// //node.GetPrepare(state, nil)
				// fmt.Println("Start ViewChange...")
				// var lastCommittedMsg *consensus.PrepareMsg = nil
				// msgTotalCnt := len(node.CommittedMsgs)
				// if msgTotalCnt > 0 {
				// 	lastCommittedMsg = node.CommittedMsgs[msgTotalCnt - 1]
				// }

				// if msgTotalCnt == 0 ||
				// 	 lastCommittedMsg.SequenceID < state.GetSequenceID() {
				// 	//startviewchange
				// 	node.IsViewChanging = true
				// 	// Broadcast view change message.
				// 	node.MsgError <- []error{ctx.Err()}
				// 	if state.GetSequenceID() == int64(1) {
				// 		fmt.Printf("&&&&&&&&&&&&&&&&&&& state.GetSequenceID %d &&&&&&&&&&&&&&&&&&\n",state.GetSequenceID())
				// 		node.StartViewChange()
				// 	}
				// }
				//return
			}
		}
	}()
}
func (node *Node) GetPrepare(state consensus.PBFT, ReqPrePareMsgs *consensus.ReqPrePareMsgs) {
	prepareMsg := ReqPrePareMsgs.PrepareMsg
	requestMsg := ReqPrePareMsgs.RequestMsg
	fmt.Printf("[GetPrepare] to %s from %s sequenceID: %d\n", 
						node.MyInfo.NodeID, prepareMsg.NodeID, prepareMsg.SequenceID)
	// When receive Prepare, save current time
	//state.SetReceivePrepareTime(time.Now())
	voteMsg, err := state.Prepare(prepareMsg, requestMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	//Check VoteMsg created
	if voteMsg.SequenceID == 0 {
		return
	}

	// Stop prepare phase and start vote phase if it is not committed
	if node.Committed[prepareMsg.SequenceID] == 1 {
		// Stop prepare phase and execute the sequence if it is committed
		state.GetTimerStopSendChannel() <- "Prepare"
		node.MsgExecution <- prepareMsg
	} else {
		state.GetTimerStopSendChannel() <- "Prepare"
		state.GetTimerStartSendChannel() <- "Vote"
	}

	// Log last sequence id for checkpointing
	atomic.AddInt64(&node.Prepared[prepareMsg.SequenceID],1)

	// Attach node ID to the message and broadcast voteMsg..
	voteMsg.NodeID = node.MyInfo.NodeID
	node.Broadcast(voteMsg, "/vote")

	// Start next sequence thread if does not exists
	node.StartThreadIfNotExists(prepareMsg.SequenceID + 1)

	if prepareMsg.Seed != -1 {
		//log.Println("Prepare for next Epoch",prepareMsg.Seed)
		//node.setNewSeedList(prepareMsg.Seed)
	}


}
func (node *Node) GetVote(state consensus.PBFT, voteMsg *consensus.VoteMsg) {
	fmt.Printf("[GetVote] to %s from %s sequenceID: %d\n", 
					node.MyInfo.NodeID, voteMsg.NodeID, voteMsg.SequenceID)

	collateMsg, err := state.Vote(voteMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	// Check COLLATE message created.
	if collateMsg.SequenceID == 0 {
		return
	}

	switch collateMsg.MsgType {
	// Stop vote phase and start collate phase if it is not committed
	case consensus.UNCOMMITTED:
		state.GetTimerStopSendChannel() <- "Vote"
		state.GetTimerStartSendChannel() <- "Collate"
	// Stop vote phase and execute the sequence if it is committed
	case consensus.COMMITTED:
		state.GetTimerStopSendChannel() <- "Vote"
		if node.Prepared[voteMsg.SequenceID] == 1 {
			node.MsgExecution <- state.GetPrepareMsg()
		}
		// Log last sequence id for checkpointing
		atomic.AddInt64(&node.Committed[voteMsg.SequenceID], 1)
	}

	// Attach node ID to the message
	collateMsg.NodeID = node.MyInfo.NodeID
	node.Broadcast(collateMsg, "/collate")
}
func (node *Node) GetCollate(state consensus.PBFT, collateMsg *consensus.CollateMsg) {
	fmt.Printf("[GetCollate] to %s from %s sequenceID: %d TYPE : %d \n", 
					node.MyInfo.NodeID, collateMsg.NodeID, collateMsg.SequenceID, collateMsg.MsgType)
	newCollateMsg, err := state.Collate(collateMsg)
	if err != nil {
		node.MsgError <- []error{err}
	}

	// Check COLLATE message created
	if newCollateMsg.SequenceID == 0 {
		return
	}

	// Try to stop current phase timer
	state.GetTimerStopSendChannel() <- "Collate"

	// Log last sequence id for checkpointing
	atomic.AddInt64(&node.Committed[collateMsg.SequenceID], 1)
	if node.Prepared[collateMsg.SequenceID] == 1 {
		node.MsgExecution <- state.GetPrepareMsg()
	}
	// Attach node ID to the message and broadcast collateMsg..
	newCollateMsg.NodeID = node.MyInfo.NodeID
	node.Broadcast(newCollateMsg, "/collate")
}
func (node *Node) createState(seqID int64) consensus.PBFT {
	// TODO: From TOCS: To guarantee exactly once semantics,
	// replicas discard requests whose timestamp is lower than
	// the timestamp in the last reply they sent to the client.
	return consensus.CreateState(node.View.ID, node.MyInfo.NodeID, len(node.NodeTable), seqID)
}
func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance:
			if !node.IsViewChanging {
				node.MsgDelivery <- msg
			}
		case viewmsg := <-node.ViewMsgEntrance:
			node.MsgDelivery <- viewmsg
		}
	}
}
func (node *Node) StartThreadIfNotExists(seqID int64) consensus.PBFT {
	node.StatesMutex.Lock()
	state := node.States[seqID]
	if state == nil {
		node.States[seqID] = node.createState(seqID)
		state = node.States[seqID]
		node.StatesMutex.Unlock()
		node.startTransitionWithDeadline(seqID, state)
		state.GetTimerStartSendChannel() <- "ViewChange"
		state.GetTimerStartSendChannel() <- "Prepare"
	}else {
		node.StatesMutex.Unlock()
	}
	return node.States[seqID]
}
func (node *Node) resolveMsg() {
	for {
		var state consensus.PBFT
		var err error = nil
		msgDelivered := <-node.MsgDelivery
		// Resolve the message.
		switch msg := msgDelivered.(type) {
		// Signature check is already done at proxyserver receiveloop..
		// Do we have to check bizantine check at receiveloop, too??
		//if node.Prepared[msg.PrepareMsg.SequenceID] == 1 &&
		// 			node.isBizantine(msg.PrepareMsg.NodeID) {
		case *consensus.ReqPrePareMsgs:
			if node.Prepared[msg.PrepareMsg.SequenceID] == 1{
				continue
			}
			state = node.StartThreadIfNotExists(msg.PrepareMsg.SequenceID)
			state.GetMsgSendChannel() <- msg

		case *consensus.VoteMsg:
			if node.Committed[msg.SequenceID] >= 1 {
				continue
			}
			state = node.StartThreadIfNotExists(msg.SequenceID)
			state.GetMsgSendChannel() <- msg
		case *consensus.CollateMsg:
			if node.Committed[msg.SequenceID] >= 1 {
			 	continue
			}
			state = node.StartThreadIfNotExists(msg.SequenceID)
			state.GetMsgSendChannel() <- msg
		//case *consensus.CheckPointMsg:
		//	node.GetCheckPoint(msg)
		case *consensus.ViewChangeMsg:
			node.GetViewChange(msg)
		case *consensus.NewViewMsg:
			node.GetNewView(msg)
		}
		if err != nil {
			// Print error.
			node.MsgError <- []error{err}
			// Send message into dispatcher.
		//	node.MsgDelivery <- msgDelivered
		}
	}
}
func (node *Node) executeMsg() {
	pairs := make(map[int64]*consensus.PrepareMsg)
	for {
		prepareMsg := <- node.MsgExecution
		node.States[prepareMsg.SequenceID].GetTimerStopSendChannel() <- "ViewChange"
		pairs[prepareMsg.SequenceID] = prepareMsg
		for {
			var lastSequenceID int64
			// Find the last committed message.
			msgTotalCnt := int64(len(node.CommittedMsgs))
			if msgTotalCnt > 0 {
				lastCommittedMsg := node.CommittedMsgs[msgTotalCnt]
				lastSequenceID = lastCommittedMsg.SequenceID
			} else {
				lastSequenceID = 0
			}
			// Stop execution if the message for the
			// current sequence is not ready to execute.
			p := pairs[lastSequenceID + 1]
			
			if p == nil {
				break
			}
			// Add the committed message in a private log queue
			// to print the orderly executed messages.
			node.CommittedMsgs[int64(lastSequenceID + 1)] = prepareMsg
			fmt.Println("[STAGE-DONE] Commit SequenceID : ",lastSequenceID + 1)
			node.StableCheckPoint = lastSequenceID + 1
			node.StatesMutex.Lock()
			if node.States[node.StableCheckPoint]!=nil && node.States[node.StableCheckPoint].GetReqMsg() != nil {
				fmt.Println("[EXECUTE TIME] PREPARE : ", time.Since(node.States[node.StableCheckPoint].GetReceivePrepareTime()))
				fmt.Println("[EXECUTE TIME] REQUEST : ", time.Since(time.Unix(0, node.States[node.StableCheckPoint].GetReqMsg().Timestamp)))
			} else {
				fmt.Println("[EXECUTE TIME] PREPARE : NULL Message came in!")
				fmt.Println("[EXECUTE TIME] REQUEST : NULL Message Came in!")
			}
			ch := node.States[node.StableCheckPoint].GetMsgExitSendChannel()
			ch <- 0
			node.StatesMutex.Unlock()
			// TODO: execute appropriate operation.


			/*
			nCheckPoint := node.CheckPointSendPoint + periodCheckPoint
			msgTotalCnt1 := len(node.CommittedMsgs)

			if node.CommittedMsgs[msgTotalCnt1 - 1].SequenceID ==  nCheckPoint{
				node.CheckPointSendPoint = nCheckPoint

				SequenceID := node.CommittedMsgs[len(node.CommittedMsgs) - 1].SequenceID
				checkPointMsg, _ := node.getCheckPointMsg(SequenceID, node.MyInfo.NodeID, node.CommittedMsgs[msgTotalCnt1 - 1])
				LogStage("CHECKPOINT", false)
				node.Broadcast(checkPointMsg, "/checkpoint")
				node.CheckPoint(checkPointMsg)
			}*/

			// delete(pairs, lastSequenceID + 1)
			
		}

		// Print all committed messages.
		/*
		for _, v := range committedMsgs {
			digest, _ := consensus.Digest(v.RequestMsg.Data)
			fmt.Printf("***committedMsgs[%d]: clientID=%s, operation=%s, timestamp=%d, data(digest)=%s***\n",
			           v.RequestMsg.SequenceID, v.RequestMsg.ClientID, v.RequestMsg.Operation, v.RequestMsg.Timestamp, digest)
		}
		*/
	}
}
func (node *Node) sendMsg() {
	sem := make(chan bool, MaxOutboundConnection)

	for {
		msg := <-node.MsgOutbound
		// Goroutine for concurrent broadcast() with timeout
		sem <- true
		go func() {
			defer func() { <-sem }()
			errCh := make(chan error, 1)

			// Goroutine for concurrent broadcast()
			go func() {
				broadcast(errCh, msg.IP, msg.Msg, msg.Path, node.PrivKey)
			}()
			select {
			case err := <-errCh:
				if err != nil {
					node.MsgError <- []error{err}
					// TODO: view change.
				}
			}
		}()
	}
}
func (node *Node) logErrorMsg() {
	coolingMsgLeft := CoolingTotalErrMsg

	for {
		errs := <-node.MsgError
		for _, err := range errs {
			coolingMsgLeft--
			if coolingMsgLeft == 0 {
				fmt.Printf("%d error messages detected! cool down for %d milliseconds\n",
				           CoolingTotalErrMsg, CoolingTime / time.Millisecond)
				time.Sleep(CoolingTime)
				coolingMsgLeft = CoolingTotalErrMsg
			}
			fmt.Println(err)
		}
	}
}
func (node *Node) getState(sequenceID int64) (consensus.PBFT, error) {
	node.StatesMutex.RLock()
	state := node.States[sequenceID]
	node.StatesMutex.RUnlock()

	if state == nil {
		return nil, fmt.Errorf("State for sequence number %d has not created yet.", sequenceID)
	}

	return state, nil
}