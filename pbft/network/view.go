package network

import (
	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
	"fmt"
	//"time"
	"sync/atomic"
	"unsafe"
)

func (node *Node) StartViewChange() {
	// Start_ViewChange
	LogStage("ViewChange", false)

	// Create SetP.
	setp := node.CreateSetP()

	// Create ViewChangeMsg.
	viewChangeMsg := node.CreateViewChangeMsg(setp)

	fmt.Printf("++++++++++++++++++++ I'm %s  \n", viewChangeMsg.NodeID)

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
	var nextviewid = node.View.ID + 1

	vcs = node.VCStates[viewchangeMsg.NextViewID]
	// Create a view state if it does not exist.
	for vcs == nil {
		vcs = consensus.CreateViewChangeState(node.MyInfo.NodeID, len(node.NodeTable), nextviewid, node.StableCheckPoint)
		// Register state into node
		node.VCStatesMutex.Lock()
		node.VCStates[viewchangeMsg.NextViewID] = vcs
		node.VCStatesMutex.Unlock()

		// Assign new VCState if node did not create the state.
		if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(node.VCStates[viewchangeMsg.NextViewID])), unsafe.Pointer(nil), unsafe.Pointer(vcs)) {
			vcs = node.VCStates[viewchangeMsg.NextViewID]
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

	// TODO: changeleader
	var nextPrimary = node.getPrimaryInfoByID(nextviewid)
	if newViewMsg == nil || node.MyInfo != nextPrimary {
		return
	}

	// Change View and Primary.
	node.updateView(newViewMsg.NextViewID)

	// Fill all the fields of NEW-VIEW message.
	//var max_s int64
	var min_s int64
	//max_s, min_s  = node.fillNewViewMsg(newViewMsg)
	min_s = node.fillNewViewMsg(newViewMsg)

	//newViewMsg.Max_S = max_s
	newViewMsg.Min_S = min_s

	// for i := node.StableCheckPoint + 1; i <= int64(len(newViewMsg.SetPrepareMsgs)); i++ {
	// 	fmt.Println("************************************************************************")
	// 	fmt.Println(newViewMsg.SetPrepareMsgs[i])
	// }
	fmt.Println(newViewMsg.PrepareMsg)
	LogStage("NewView", false)

	LogMsg(newViewMsg)

	node.Broadcast(newViewMsg, "/newview")
	LogStage("NewView", true)

}

func (node *Node) fillNewViewMsg(newViewMsg *consensus.NewViewMsg) (int64){
	// Search min_s the sequence number of the latest stable checkpoint and
	// max_s the highest sequence number in a prepare message in V.
	var min_s int64 = 0

	fmt.Println("***********************N E W V I E W***************************")
	for _, vcm := range newViewMsg.SetViewChangeMsgs {
		if min_s < vcm.StableCheckPoint {
			min_s = vcm.StableCheckPoint
		}

		// for seq, voteSet := range vcm.SetP { //if max_s get seq, it could be problem that max_s get pre-prepare's sequenceID
		// 	if seq < max_s {
		// 		continue
		// 	}
		// 	for _, voteMsg := range voteSet.VoteMsgs {
		// 		if voteMsg == nil {
		// 			continue
		// 		}
		// 		if max_s < voteMsg.SequenceID {
		// 			max_s = voteMsg.SequenceID
		// 		}
		// 	}
		// }
	}

	//fmt.Println("min_s ", min_s, "max_s", max_s)
	fmt.Println("min_s ", min_s)

	// Create SetPrePrepareMsgs of the new-view for redo
	// only if a preprepare message of the SetPrePrepareMsgs with sequence number seq is nil.
	// newMap := make(map[int64]*consensus.PrepareMsg)

	// for _, vcm := range newViewMsg.SetViewChangeMsgs {
	// 	for seq, setpm := range vcm.SetP {
	// 		if seq <= min_s{
	// 			continue
	// 		}
	// 		if newMap[seq] == nil {
	// 			digest := setpm.PrepareMsg.Digest
	// 			newMap[seq] = GetPrepareForNewview(newViewMsg.NextViewID, seq, digest)
	// 		}
	// 	}
	// }
	// newViewMsg.SetPrepareMsgs = newMap
	

	for _, vcm := range newViewMsg.SetViewChangeMsgs {
		for seq, _ := range vcm.SetP {
			if seq <= min_s{
				continue
			}
			//fmt.Println("========setpm.PrepareMsg.Digest ", setpm.PrepareMsg.Digest)
			//digest := setpm.PrepareMsg.Digest
			newViewMsg.PrepareMsg = GetPrepareForNewview(newViewMsg.NextViewID, min_s)
		}
	}

	return min_s
}

func (node *Node) GetNewView(newviewMsg *consensus.NewViewMsg) error {

	// TODO verify new-view message

	// Register new-view message into this node
	node.VCStatesMutex.Lock()
	node.VCStates[newviewMsg.NextViewID].NewViewMsg = newviewMsg
	node.VCStatesMutex.Unlock()

	fmt.Printf("<<<<<<<<<<<<<<<<NewView>>>>>>>>>>>>>>>>: %d by %s\n", newviewMsg.NextViewID, newviewMsg.NodeID)

	// Change View and Primary
	node.updateView(newviewMsg.NextViewID)
	node.StableCheckPoint = newviewMsg.Min_S
	// Fill missing states and messages
	node.FillHole(newviewMsg)

	/*// Redo
	for _, prePrepareMsg := NewViewMsg.SetPrePrepareMsgs {
		var state consensus.PBFT
		state, _ = node.getState(prePrepareMsg.SequenceID)
		node.GetPrePrepareMsgs(state, prePrepareMsg)
	}
	*///
	// Accept messages usign MsgEntrance channel
	node.IsViewChanging = false


	// verify view number of new-view massage
	if newviewMsg.NextViewID != node.View.ID + 1 {
		return nil
	}

	return nil
}

func (node *Node) FillHole(newviewMsg *consensus.NewViewMsg) {
	// Check the number of states
	fmt.Println("node.TotalConsensus :  ",node.TotalConsensus)

	fmt.Println("newviewMsg.Min_S : ", newviewMsg.Min_S)
	//fmt.Println("newviewMsg.Max_S : ", newviewMsg.Max_S)

	// Currunt Max sequence number of committed request
	var committedMax int64 = 0
	for seq, _ := range node.CommittedMsgs{
		if committedMax <= int64(seq) {
			committedMax = int64(seq)
		}
	}
	fmt.Println("committedMax : ", committedMax)
	for committedMax < newviewMsg.Min_S {
		var prepare consensus.PrepareMsg
		newSequenceID := committedMax
		prepare.SequenceID = newSequenceID
		prepare.ViewID = int64(0)
		prepare.Digest = ""
		prepare.EpochID = 0
		prepare.NodeID = ""

		fmt.Println("no request in node.CommittedMsgs : ", newSequenceID)
		//node.CommittedMsgs = append(node.CommittedMsgs, &request)
		committedMax += 1
	}
	// if highest sequence number of received request and state is lower than min-s,
	// node.TotalConsensus be added util min-s - 1
	for node.TotalConsensus < newviewMsg.Min_S {
		atomic.AddInt64(&node.TotalConsensus, 1)
	}

	//Fill the PrePrepare message from new-view message
	prepareMsg := newviewMsg.PrepareMsg
	fmt.Println("newview seq : ", prepareMsg.SequenceID)

	// preprepare.sequenceID could be higher than Max_S of SetPrePare
	//if seq > newviewMsg.Max_S {

	//	continue
	//}

	// node.StatesMutex.Lock()
	var state consensus.PBFT
	state, err := node.getState(prepareMsg.SequenceID)
	if err != nil {
		// Print error.
	}
	if state != nil {
		// Fill the committedMax if it is not committed
		if prepareMsg.SequenceID > committedMax {
			fmt.Println("no request in node.CommittedMsgs : ", prepareMsg.SequenceID)
				//node.CommittedMsgs = append(node.CommittedMsgs, state.GetReqMsg())
		}
			// Initalize all of logs of this state
		state.ClearMsgLogs()

			// Change the viewid, preprepare message and preprepare message's digest of the state
		node.States[prepareMsg.SequenceID] = state.Redo_SetState(newviewMsg.NextViewID, node.MyInfo.NodeID, len(node.NodeTable), prepareMsg, prepareMsg.Digest)

	} else { //if this node does not have state and a request with sequence number n (prePrepareMsg.SequenceID)
			// Fill the state of the sequence number of prePrepareMsg
		state := node.createState(0)

		state.SetSequenceID(prepareMsg.SequenceID)
		// Log REQUEST message.
		var prepare consensus.PrepareMsg
		prepare.SequenceID = prepareMsg.SequenceID
		prepare.ViewID = int64(0)
		prepare.Digest = ""
		prepare.EpochID = int64(0)
		prepare.NodeID = ""

			//state.SetReqMsg(&request)
		fmt.Println("no request in node.CommittedMsgs : ", prepareMsg.SequenceID)
			// Fill request message in the committedMax
			//node.CommittedMsgs = append(node.CommittedMsgs, state.GetReqMsg())
			
			// Change the viewid, preprepare message and preprepare message's digest of the state
		node.States[prepareMsg.SequenceID] = state.Redo_SetState(newviewMsg.NextViewID, node.MyInfo.NodeID, len(node.NodeTable), prepareMsg, prepareMsg.Digest)

		atomic.AddInt64(&node.TotalConsensus, 1)
	}
	//node.StatesMutex.Unlock()

}

func (node *Node) updateView(viewID int64) {
	node.View.ID = viewID
	node.View.Primary = node.getPrimaryInfoByID(viewID)
}

func (node *Node) isMyNodePrimary() bool {
	return node.MyInfo.NodeID == node.View.Primary.NodeID
}

func (node *Node) getPrimaryInfoByID(viewID int64) *NodeInfo {
	var Epoch int64 = 4
	viewIdx := viewID % Epoch
	return node.NodeTable[viewIdx]
}

func GetPrepareForNewview(nextviewID int64, sequenceid int64) *consensus.PrepareMsg {
	return &consensus.PrepareMsg{
		ViewID:     nextviewID,
		SequenceID: sequenceid,
		Digest:     "",
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

func (node *Node) CreateViewChangeMsg(setp map[int64]*consensus.SetPm) *consensus.ViewChangeMsg {
	// Get checkpoint message log for the latest stable checkpoint (C)
	// for this node.
	stableCheckPoint := node.StableCheckPoint
	//setc := node.CheckPointMsgsLog[stableCheckPoint]
	fmt.Println("node.StableCheckPoint : ", stableCheckPoint)
	//fmt.Println("setc",setc)

	return &consensus.ViewChangeMsg{
		NodeID: node.MyInfo.NodeID,
		NextViewID: node.View.ID + 1,
		StableCheckPoint: stableCheckPoint,
		//SetC: setc,
		SetP: setp,
	}
}

func (node *Node) ChangeLeader(){

}
