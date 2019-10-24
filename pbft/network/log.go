package network

import (
	"fmt"

	"github.com/bigpicturelabs/consensusPBFT/pbft/consensus"
)

func LogMsg(msg interface{}) {
	switch msg.(type) {
	case *consensus.RequestMsg:
		reqMsg := msg.(*consensus.RequestMsg)
		fmt.Printf("[REQUEST] ClientID: %s, Timestamp: %d, Operation: %s\n", reqMsg.ClientID, reqMsg.Timestamp, reqMsg.Operation)
	case *consensus.PrePrepareMsg:
		prePrepareMsg := msg.(*consensus.PrePrepareMsg)
		fmt.Printf("[PREPREPARE] SequenceID: %d\n", prePrepareMsg.SequenceID)
	case *consensus.VoteMsg:
		voteMsg := msg.(*consensus.VoteMsg)
		if voteMsg.MsgType == consensus.PrepareMsg {
			fmt.Printf("[PREPARE] NodeID: %s\n", voteMsg.NodeID)
		} else if voteMsg.MsgType == consensus.CommitMsg {
			fmt.Printf("[COMMIT] NodeID: %s\n", voteMsg.NodeID)
		}
	case *consensus.CheckPointMsg:
		CheckPointMsg := msg.(*consensus.CheckPointMsg)
		fmt.Printf("[CheckPointMsg] NodeID: %s\n", CheckPointMsg.NodeID)
	case *consensus.ViewChangeMsg:
		viewchangeMsg := msg.(*consensus.ViewChangeMsg)
		fmt.Printf("[ViewChangeMsg] NodeID: %s\n", viewchangeMsg.NodeID)
	}
}

func LogStage(stage string, isDone bool) {
	if isDone {
		fmt.Printf("[STAGE-DONE] %s\n", stage)
	} else {
		fmt.Printf("[STAGE-BEGIN] %s\n", stage)
	}
}
