package consensus

// Messages are TOCS style.

type RequestMsg struct {
	Timestamp  int64  `json:"timestamp"`
	ClientID   string `json:"clientID"`
	Operation  string `json:"operation"`
	Data       string `json:"data"`
	SequenceID int64  `json:"sequenceID"`
	//Adaptive BFT
	NodeID     int64  `json:"nodeID"`
	Signature  string `json:"signature"`
}

type ReplyMsg struct {
	ViewID    int64  `json:"viewID"`
	Timestamp int64  `json:"timestamp"` // same timestamp value as RequestMsg
	ClientID  string `json:"clientID"`
	NodeID    string `json:"nodeID"`
	Result    string `json:"result"`
}

//type PrePrepareMsg struct {
type PrepareMsg struct {
	ViewID     int64       `json:"viewID"`
	SequenceID int64       `json:"sequenceID"`
	RequestMsg *RequestMsg `json:"requestMsg"`
	Digest     string      `json:"digest"`
	//Adaptive BFT
	EpochID 	int64      `json:"epochID"`		//epoch
	NodeID      string     `json:"nodeID"`
	Signature 	string 	   `json:"signature"`
}


type VoteMsg struct {
	ViewID     int64  `json:"viewID"`
	SequenceID int64  `json:"sequenceID"`
	Digest     string `json:"digest"` // COMMIT message does not have digest
	NodeID     string `json:"nodeID"`
	MsgType           `json:"msgType"`
	//Adaptive BFT
	PrepareMsg 	*PrepareMsg `json:"prepareMsg"`		//prepare msg
	Signature 	string      `json:"signature"`
}

//Adaptive BFT
type CollateMsg struct {
	ReceivedPrepare		*PrepareMsg
	ReceivedVoteMsg     map[string]*VoteMsg `json:"commit_proof"`	//commit proof
	SentVoteMsg         *VoteMsg   `json:"sent_vote_msg"`	//vote msgs that i sent
	ViewID              int64      `json:"viewID"`
	SequenceID          int64      `json:"sequenceID"`
	Digest              string     `json:"digest"`
	MsgType                        `json:"msgType"`
	NodeID              string     `json:"nodeID"`
	Signature           string     `json:"signature"`
}

type CheckPointMsg struct {
	SequenceID int64  `json:"sequenceID"`
	Digest     string `json:"digest"`
	NodeID     string `json:"nodeID"`
}

type ViewChangeMsg struct {
	NodeID     string `json:"nodeID"`
	NextViewID int64  `json:"nextviewID"`
	StableCheckPoint int64 `json:"stableCheckPoint"`
	//C checkpointmsg_set 2f+1
	//SetP -> a set of preprepare + (preparemsg * 2f+1) from stablecheckpoint to the biggest sequence_num that node received
	SetP  map[int64]*SetPm	`json:"setP"`
}

type SetPm struct {
	//PrePrepareMsg *PrePrepareMsg
	PrepareMsg    *PrepareMsg
	//PrepareMsgs   map[string]*VoteMsg
	VoteMsgs      map[string]*VoteMsg
}


type NewViewMsg struct {
	NodeID     string `json:"nodeID"`
	NextViewID int64  `json:"nextviewID"`
	//V a set containing the valid ViewChageMsg
	//O a set of PrePrepareMsgs from latest stable checkpoint(min-s) in V to the highest sequence number(max-s) in a PrepareMsg in V
	//  new Primary creates a new PrePrepareMsg for view v+1 for each sequence number between min-s and max-s
}

type MsgType int
const (
	//PrepareMsg MsgType = iota
	//CommitMsg
	//Aaptive BFT
	VOTE 	MsgType = iota
	REJECT
	NULLMSG
	COMMITTED
	UNCOMMITTED
)
