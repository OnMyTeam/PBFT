package network

import (
	"fmt"
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"time"
	"sync/atomic"
	"unsafe"
	"log"
)
var seedList []string
func (node *Node) StartViewChange(sequenceID int64) {
	// Start_ViewChange
//	LogStage("ViewChange", false)
	// Create SetP.
	setp := node.CreateSetP()

	// Create ViewChangeMsg.
	viewChangeMsg := node.CreateViewChangeMsg(setp, sequenceID)

//	fmt.Printf("++++++++++++++++++++ I'm %s  \n", viewChangeMsg.NodeID)

	// VIEW-CHANGE message created by this node will be received
	// at this node as well as the other nodes.
	node.Broadcast(viewChangeMsg, "/viewchange")
	fmt.Println("Breadcast viewchange")
	LogStage("ViewChange", true)
}

func (node *Node) GetViewChange(viewchangeMsg *consensus.ViewChangeMsg) {
	var vcs *consensus.VCState

	//LogMsg(viewchangeMsg)
	fmt.Printf("++++from %s ++++++++++++++++++++ seq: %d \n", viewchangeMsg.NodeID, viewchangeMsg.SequenceID)

	// Ignore VIEW-CHANGE message if the next view id is not new.
	
	vcs = node.VCStates[viewchangeMsg.SequenceID]
	//fmt.Printf("node.NextCandidateIdx : %d\n", node.NextCandidateIdx)
	// Create a view state if it does not exist.
	for vcs == nil {
		vcs = consensus.CreateViewChangeState(node.MyInfo.NodeID, len(node.NodeTable), node.NextCandidateIdx, node.StableCheckPoint, viewchangeMsg.SequenceID)
		// Register state into node
		node.VCStatesMutex.Lock()
		node.VCStates[viewchangeMsg.SequenceID] = vcs
		node.VCStatesMutex.Unlock()

		// Assign new VCState if node did not create the state.
		if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(node.VCStates[viewchangeMsg.SequenceID])), unsafe.Pointer(nil), unsafe.Pointer(vcs)) {
			vcs = node.VCStates[viewchangeMsg.SequenceID]
		}
	}

	newViewMsg, err := vcs.ViewChange(viewchangeMsg)
	if err != nil {
		fmt.Println(err)
		return
	}

	if newViewMsg != nil {
		node.IsViewChanging = true
		time.Sleep(time.Millisecond * 200)
		fmt.Println("+++++ node.IsViewChanging", node.IsViewChanging)

		var totalcon int64 = node.TotalConsensus	
		for i := int64(newViewMsg.SequenceID); i <= totalcon; i++ {

			state, _ := node.getState(i)
			
			ch := state.GetMsgExitSendChannel()
			if ch != nil {
				ch <- 0
			}
			if node.CommittedMsgs[i] != nil {
				delete(node.CommittedMsgs, i)
			}
			if node.Committed[i] != 0 {
				node.Committed[i] = 0
			}
			if node.Prepared[i] != 0 {
				node.Prepared[i] = 0
			}
			delete(node.States, i)
			atomic.AddInt64(&node.TotalConsensus, -1)
		}

	}

	var nextPrimary = node.getPrimaryInfoByID(node.NextCandidateIdx)

	if  node.MyInfo == nextPrimary && newViewMsg != nil {

		newViewMsg.Min_S = node.FindStableCheckpoint(newViewMsg)
		newViewMsg.EpochID = newViewMsg.Min_S / 10

		LogMsg(newViewMsg)
		fmt.Println("+++++ newView_Broadcast")
		node.Broadcast(newViewMsg, "/newview")
	}
}

func (node *Node) FindStableCheckpoint(newViewMsg *consensus.NewViewMsg) (int64){
	// Search min_s the sequence number of the latest stable checkpoint and
	// max_s the highest sequence number in a prepare message in V.
	var min_s int64 = 0

	//fmt.Println("***********************N E W V I E W***************************")
	for _, vcm := range newViewMsg.SetViewChangeMsgs {
		if min_s < vcm.StableCheckPoint {
			min_s = vcm.StableCheckPoint
		}
	}
//	fmt.Println("min_s ", min_s)
	return min_s
}

func (node *Node) GetNewView(newviewMsg *consensus.NewViewMsg) {
	// TODO verify new-view message
	fmt.Printf("<<<<<<<<<<<<<<<<GetNewView seq %d >>>>>>>>>>>>>>>>: NextCandidateIdx: %d by %s\n", newviewMsg.SequenceID , newviewMsg.NextCandidateIdx, newviewMsg.NodeID)

	node.IsViewChanging = true
	time.Sleep(time.Millisecond * 200)
	
	var totalcon int64 = node.TotalConsensus	
	for i := int64(newviewMsg.SequenceID); i <= totalcon; i++ {
	//	fmt.Println("+++++ i,  node.TotalConsensus", i, node.TotalConsensus)
		
		state, _ := node.getState(i)
		
		ch := state.GetMsgExitSendChannel()
		if ch != nil {
			ch <- 0
		}
		if node.CommittedMsgs[i] != nil {
			delete(node.CommittedMsgs, i)
		}
		if node.Committed[i] != 0 {
			node.Committed[i] = 0
		}
		if node.Prepared[i] != 0 {
			node.Prepared[i] = 0
		}
		delete(node.States, i)
		atomic.AddInt64(&node.TotalConsensus, -1)
	}

	node.NextCandidateIdx = newviewMsg.NextCandidateIdx

	var vcs *consensus.VCState
	vcs = node.VCStates[newviewMsg.SequenceID]
	
	// Create a view state if it does not exist.
	for vcs == nil {
		vcs = consensus.CreateViewChangeState(node.MyInfo.NodeID, len(node.NodeTable), node.NextCandidateIdx, node.StableCheckPoint, newviewMsg.SequenceID)
		// Register state into node
		node.VCStatesMutex.Lock()
		node.VCStates[newviewMsg.SequenceID] = vcs
		node.VCStatesMutex.Unlock()

		// Assign new VCState if node did not create the state.
		if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(node.VCStates[newviewMsg.SequenceID])), unsafe.Pointer(nil), unsafe.Pointer(vcs)) {
			vcs = node.VCStates[newviewMsg.SequenceID]
		}
	}
	
	// for _, vcm := range newviewMsg.SetViewChangeMsgs {
	// 	node.GetViewChange(vcm)
	// }
	//////////////////////////////////////////////////////////////////////////

	// Register new-view message into this node
	node.VCStatesMutex.Lock()
	node.VCStates[newviewMsg.SequenceID].NewViewMsg = newviewMsg
	node.VCStatesMutex.Unlock()

	// Fill missing states and messages
	node.FillHole(newviewMsg)
	// Change View and Primary
	node.StableCheckPoint = newviewMsg.Min_S
	node.EpochID = newviewMsg.EpochID

//	fmt.Println("node.NextCandidateIdex: ",node.NextCandidateIdx)

//	fmt.Println("node.TotalConsensus:  ",node.TotalConsensus)

///	fmt.Printf("node.StableCheckPoint: %d , newviewMsg.Min_S: %d\n", node.StableCheckPoint, newviewMsg.Min_S)


//	fmt.Println("newviewMsg.PrepareMsg: ", newviewMsg.PrepareMsg)
	

	node.updateViewID(newviewMsg.SequenceID-1)
	node.updateEpochID(newviewMsg.SequenceID-1)
				
	fmt.Println("node.NextCandidateIdx: ", node.NextCandidateIdx)
	primaryNode := node.NodeTable[node.NextCandidateIdx]

				
	fmt.Println("[VIEWCHANGE_DONE] ",",",newviewMsg.SequenceID,",",time.Since(node.VCStates[newviewMsg.SequenceID].GetReceiveViewchangeTime()))

	atomic.AddInt64(&node.NextCandidateIdx, 1)

	if newviewMsg.SequenceID % 10 == 0 {
	//	node.VCStates = make(map[int64]*consensus.VCState)
		node.NextCandidateIdx = 7
	}

	node.StartThreadIfNotExists(newviewMsg.SequenceID)

	node.IsViewChanging = false

	if primaryNode.NodeID == node.MyInfo.NodeID {
		var seed int64 = -1	

		data := make([]byte, 1 << 20)
		for i := range data {
			data[i] = 'A'
		}
		data[len(data)-1]=0

		prepareMsg := PrepareMsgMaking("Op1", "Client1", data, 
			node.View.ID,int64(newviewMsg.SequenceID),
			node.MyInfo.NodeID, int(seed), node.EpochID)
					
			log.Printf("Broadcasting dummy message from %s, sequenceId: %d, epochId: %d, viewId: %d",
				node.MyInfo.NodeID, int64(newviewMsg.SequenceID), node.EpochID, node.View.ID)
		// Broadcast the dummy message.
		errCh := make(chan error, 1)
					
		fmt.Println("[StartPrepare]", "seqID / ",newviewMsg.SequenceID,"/", time.Now().UnixNano())
		node.Broadcast(prepareMsg, "/prepare")
		fmt.Println("[StartPrepare] After Broadcast!")

		err := <-errCh
		if err != nil {
			log.Println(err)
		}

	}

}

func (node *Node) FillHole(newviewMsg *consensus.NewViewMsg) {
	// Check the number of states
//	fmt.Println("node.TotalConsensus :  ",node.TotalConsensus)

//	fmt.Println("newviewMsg.Min_S : ", newviewMsg.Min_S)
	//fmt.Println("newviewMsg.Max_S : ", newviewMsg.Max_S)

	// Currunt Max sequence number of committed request
	var committedMax int64 = 0
	for seq, _ := range node.CommittedMsgs{
		if committedMax <= int64(seq) {
			committedMax = int64(seq)
		}
	}
//	fmt.Println("committedMax : ", committedMax)
	for committedMax <= newviewMsg.Min_S {
		var prepare consensus.PrepareMsg
		newSequenceID := committedMax
		prepare.SequenceID = newSequenceID
		prepare.ViewID = int64(0)
		prepare.Digest = ""
		prepare.EpochID = newSequenceID / int64(len(node.NodeTable))
		prepare.NodeID = ""


//		fmt.Println("no request in node.CommittedMsgs : ", newSequenceID)
		node.CommittedMsgs[newSequenceID] = &prepare
		committedMax += 1
	}
	// if highest sequence number of received request and state is lower than min-s,
	// node.TotalConsensus be added util min-s - 1

	for node.TotalConsensus < newviewMsg.Min_S {
		atomic.AddInt64(&node.TotalConsensus, 1)
	}

//	fmt.Println("+++++++++++++++++++FILLHOLE DONE++++++++++++++++++++")

}

func (node *Node) updateEpochID(sequenceID int64) {
	epochID := sequenceID / int64(len(node.NodeTable))
	node.EpochID = epochID
}

func (node *Node) updateViewID(viewID int64) {
	var participant int64 = int64(len(node.NodeTable))
	node.View.ID = viewID % participant
	node.View.Primary = node.getPrimaryInfoByID(node.View.ID)
}

func (node *Node) isMyNodePrimary() bool {
	return node.MyInfo.NodeID == node.View.Primary.NodeID
}

func(node *Node) setNewSeedList(seedNo int) int {
	node.NodeTable = node.SeedNodeTables[seedNo]
	fmt.Println("New seed is ",seedNo)
	return seedNo
}

func (node *Node) getPrimaryInfoByID(viewID int64) *NodeInfo {
	viewIdx := viewID 
	if viewIdx > int64(len(node.NodeTable)) {
		log.Printf("Fail this Consensus by %s\n", node.MyInfo.NodeID)
	}
	return node.NodeTable[viewIdx]
}

func GetPrepareForNewview(nextviewID int64, sequenceid int64, digest string) *consensus.PrepareMsg {
	return &consensus.PrepareMsg {
		ViewID:     nextviewID,
		SequenceID: sequenceid,
		Digest:     digest,
	}
}

// Create a set of PreprepareMsg and PrepareMsgs for each sequence number.
func (node *Node) CreateSetP() map[int64]*consensus.SetPm {
	setp := make(map[int64]*consensus.SetPm)

	node.StatesMutex.RLock()
	for seqID, state := range node.States {
		var setPm consensus.SetPm
		setPm.PrepareMsg = state.GetPrepareMsg()
		setPm.VoteMsgs = state.GetVoteMsgs()
		setp[seqID] = &setPm
	}
	node.StatesMutex.RUnlock()

	return setp
}

func (node *Node) CreateViewChangeMsg(setp map[int64]*consensus.SetPm, sequenceID int64) *consensus.ViewChangeMsg {
	// Get checkpoint message log for the latest stable checkpoint (C)
	// for this node.
	stableCheckPoint := node.StableCheckPoint
	//setc := node.CheckPointMsgsLog[stableCheckPoint]
//	fmt.Println("node.StableCheckPoint : ", stableCheckPoint)
	//fmt.Println("setc",setc)

	//committeeNum := int64(7)

	return &consensus.ViewChangeMsg{
		NodeID: node.MyInfo.NodeID,
		SequenceID: sequenceID,
		NextCandidateIdx: node.NextCandidateIdx,
		StableCheckPoint: stableCheckPoint,
		//SetC: setc,
		SetP: setp,
	}
}

func (node *Node) ChangeLeader(){

}
