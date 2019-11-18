package network

import (
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"fmt"
	"time"
	"sync/atomic"
	"unsafe"
)

func (node *Node) StartViewChange() {
	// Start_ViewChange
//	LogStage("ViewChange", false)

	var lastCommittedMsg *consensus.PrepareMsg = nil
	msgTotalCnt := int64(len(node.CommittedMsgs))
	if msgTotalCnt > 0 {
		lastCommittedMsg = node.CommittedMsgs[msgTotalCnt]
	}

	var totalcon int64 = node.TotalConsensus	
	for i := int64(lastCommittedMsg.SequenceID+1); i <= totalcon; i++ {
//		fmt.Println("+++++ i,  node.TotalConsensus", i, node.TotalConsensus)
		
		state, _ := node.getState(i)
		if i != lastCommittedMsg.SequenceID+1 {
//			fmt.Println("state. GetSequenceID()  ",state.GetSequenceID())
			ch2 := state.GetMsgExitSendChannel()
			ch2 <- 0
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

	// Create SetP.
	setp := node.CreateSetP()

	// Create ViewChangeMsg.
	viewChangeMsg := node.CreateViewChangeMsg(setp)

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
	fmt.Printf("++++from %s ++++++++++++++++++++ \n", viewchangeMsg.NodeID)

	// Ignore VIEW-CHANGE message if the next view id is not new.
	
//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	vcs = node.VCStates[node.NextCandidateIdx]
	fmt.Printf("node.NextCandidateIdx : %d\n", node.NextCandidateIdx)
	// Create a view state if it does not exist.
	for vcs == nil {
		vcs = consensus.CreateViewChangeState(node.MyInfo.NodeID, len(node.NodeTable), node.NextCandidateIdx, node.StableCheckPoint)
		// Register state into node
		node.VCStatesMutex.Lock()
		node.VCStates[node.NextCandidateIdx] = vcs
		node.VCStatesMutex.Unlock()

		// Assign new VCState if node did not create the state.
		if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(node.VCStates[node.NextCandidateIdx])), unsafe.Pointer(nil), unsafe.Pointer(vcs)) {
			vcs = node.VCStates[node.NextCandidateIdx]
		}
	}

	newViewMsg, err := vcs.ViewChange(viewchangeMsg)
	if err != nil {
		fmt.Println(err)
		return
	}

	// From OSDI: When the primary of view v + 1 receives 2f valid
	// view-change messages for view v + 1 from other replicas,
	// it multicasts a NEW-VIEW message to all other replicas.
	node.VCStates[node.NextCandidateIdx].SetReceiveViewchangeTime(time.Now())
	
	// TODO: changeleader
	var nextPrimary = node.getPrimaryInfoByID(node.NextCandidateIdx)
	if newViewMsg == nil || node.MyInfo != nextPrimary {
		fmt.Println("Before Return")
		return
	}

	// Change View and Primary.
	//node.updateView(node.NextCandidateIdex)

	// Fill all the fields of NEW-VIEW message.
	//var max_s int64
	var min_s int64
	//max_s, min_s  = node.fillNewViewMsg(newViewMsg)
	min_s = node.fillNewViewMsg(newViewMsg)

	newViewMsg.Min_S = min_s

	//fmt.Println(newViewMsg.PrepareMsg)
//	LogStage("NewView", false)

	LogMsg(newViewMsg)

	node.Broadcast(newViewMsg, "/newview")
//	LogStage("NewView", true)

}

func (node *Node) fillNewViewMsg(newViewMsg *consensus.NewViewMsg) (int64){
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

	newViewMsg.EpochID = min_s / 4

	return min_s
}

func (node *Node) GetNewView(newviewMsg *consensus.NewViewMsg) error {

	// TODO verify new-view message
	fmt.Printf("<<<<<<<<<<<<<<<<GetNewView>>>>>>>>>>>>>>>>: NextCandidateIdx: %d by %s\n", newviewMsg.NextCandidateIdx, newviewMsg.NodeID)

	//////////////////////////////////////////////////////////////////////////
	var totalcon int64 = node.TotalConsensus
//	fmt.Println("+++++ node.TotalConsensus", totalcon)

	var lastCommittedMsg *consensus.PrepareMsg = nil
	msgTotalCnt := int64(len(node.CommittedMsgs))
	if msgTotalCnt > 0 {
		lastCommittedMsg = node.CommittedMsgs[msgTotalCnt]
	}

	if node.IsViewChanging == false || newviewMsg.Min_S+1 < totalcon {
		node.IsViewChanging = true

		for i := newviewMsg.Min_S+1; i <= totalcon; i++ {
		
			state, _ := node.getState(i)
			if i != lastCommittedMsg.SequenceID+1 {
//				fmt.Println("state. GetSequenceID()  ",state.GetSequenceID())
				ch2 := state.GetMsgExitSendChannel()
				ch2 <- 0
			}
			delete(node.States, i)
			if node.CommittedMsgs[i] != nil {
				delete(node.CommittedMsgs, i)
			}
			if node.Committed[i] != 0 {
				node.Committed[i] = 0
			}
			if node.Prepared[i] != 0 {
				node.Prepared[i] = 0
			}
			atomic.AddInt64(&node.TotalConsensus, -1)
		}

	}
	
	//////////////////////////////////////////////////////////////////////////

	// Register new-view message into this node
	node.VCStatesMutex.Lock()
	node.VCStates[node.NextCandidateIdx].NewViewMsg = newviewMsg
	node.VCStatesMutex.Unlock()

	// Fill missing states and messages
	node.FillHole(newviewMsg)
	//fmt.Println("==================After fillhole=============== ")
	// Change View and Primary
	//node.updateView(node.NextCandidateIdex)
	node.StableCheckPoint = newviewMsg.Min_S
	

//	fmt.Println("node.NextCandidateIdex: ",node.NextCandidateIdx)

//	fmt.Println("node.TotalConsensus:  ",node.TotalConsensus)

///	fmt.Printf("node.StableCheckPoint: %d , newviewMsg.Min_S: %d\n", node.StableCheckPoint, newviewMsg.Min_S)


//	fmt.Println("newviewMsg.PrepareMsg: ", newviewMsg.PrepareMsg)
	
	viewchangechannel := ViewChangeChannel{
		Min_S: newviewMsg.Min_S,
		VCSCheck: true,
	}

	node.ViewChangeChan <- viewchangechannel


	// verify view number of new-view massage
	// if newviewMsg.nextCandidateIdx != node.View.ID + 1 {
	// 	return nil
	// }

	return nil
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
		prepare.EpochID = 0
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
	epochID := sequenceID / 10
	node.EpochID = epochID
}

func (node *Node) updateView(viewID int64) {
	var participant int64 = 10
	node.View.ID = viewID % participant
	node.View.Primary = node.getPrimaryInfoByID(node.View.ID)
}


func (node *Node) isMyNodePrimary() bool {
	return node.MyInfo.NodeID == node.View.Primary.NodeID
}

func (node *Node) getPrimaryInfoByID(viewID int64) *NodeInfo {
	viewIdx := viewID 
	return node.NodeTable[viewIdx]
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

func (node *Node) CreateViewChangeMsg(setp map[int64]*consensus.SetPm) *consensus.ViewChangeMsg {
	// Get checkpoint message log for the latest stable checkpoint (C)
	// for this node.
	stableCheckPoint := node.StableCheckPoint
	//setc := node.CheckPointMsgsLog[stableCheckPoint]
//	fmt.Println("node.StableCheckPoint : ", stableCheckPoint)
	//fmt.Println("setc",setc)

	//committeeNum := int64(7)

	return &consensus.ViewChangeMsg{
		NodeID: node.MyInfo.NodeID,
		NextCandidateIdx: node.NextCandidateIdx,
		StableCheckPoint: stableCheckPoint,
		//SetC: setc,
		SetP: setp,
	}
}

func (node *Node) ChangeLeader(){

}
