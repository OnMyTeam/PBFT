package consensus

// Messages are TOCS style.

type RequestMsg struct {
	Timestamp  int64  `json:"timestamp"`
	ClientID   string `json:"clientID"`
	Operation  string `json:"operation"`
	Data       string `json:"data"`
	SequenceID int64  `json:"sequenceID"`
}

type ReplyMsg struct {
	ViewID    int64  `json:"viewID"`
	Timestamp int64  `json:"timestamp"`
	ClientID  string `json:"clientID"`
	NodeID    string `json:"nodeID"`
	Result    string `json:"result"`
}

//type PrePrepareMsg struct {
type PrepareMsg struct {
	ViewID     int64       `json:"viewID"`
	SequenceID int64       `json:"sequenceID"`
	Digest     string      `json:"digest"`
	EpochID 	int64      `json:"epochID"`
	NodeID      string     `json:"nodeID"`
}

type VoteMsg struct {
	ViewID     int64  		`json:"viewID"`
	SequenceID int64  		`json:"sequenceID"`
	Digest     string 		`json:"digest"`
	NodeID     string 		`json:"nodeID"`
	MsgType           		`json:"msgType"`
}

//Adaptive BFT
type CollateMsg struct {
	//ReceivedPrepare		*PrepareMsg 		`json:"received_prepare`
	ReceivedVoteMsg     map[string]*VoteMsg `json:"commit_proof"`
	SentVoteMsg         *VoteMsg   			`json:"sent_vote_msg"`
	ViewID              int64      			`json:"viewID"`
	SequenceID          int64      			`json:"sequenceID"`
	Digest              string     			`json:"digest"`
	MsgType             				`json:"msgType"`
	NodeID              string     			`json:"nodeID"`
}
type ReqPrePareMsgs struct {
	RequestMsg *RequestMsg 
	PrepareMsg *PrepareMsg 
}
type SignatureMsg struct {
	// signature
	//Signature []byte `json:"signature"`
	//R *big.Int `json:"r"`
	//S *big.Int `json:"s"`
	MsgType		string 	`json:"msgType"`

	// any consensus messages
	MarshalledMsg []byte `json:"marshalledmsg"`
}


type CheckPointMsg struct {
	SequenceID int64  `json:"sequenceID"`
	Digest     string `json:"digest"`
	NodeID     string `json:"nodeID"`
}

type ViewChangeMsg struct {
	NodeID     string `json:"nodeID"`
	NextCandidateIdx int64  `json:"nextcandidateIdx"`
	StableCheckPoint int64 `json:"stableCheckPoint"`
	//SetC map[string]*CheckPointMsg `json:"setC"`//C checkpointmsg_set 2f+1
	SetP  map[int64]*SetPm	`json:"setP"`//SetP -> a set of preprepare + (preparemsg * 2f+1) from stablecheckpoint to the biggest sequence_num that node received
}

type SetPm struct {
	PrepareMsg *PrepareMsg
	VoteMsgs   map[string]*VoteMsg
}


type NewViewMsg struct {
	NodeID     string `json:"nodeID"`
	NextCandidateIdx int64  `json:"nextcandidateIdx"`
	EpochID    int64 `json:"epochID"`
	SetViewChangeMsgs map[string]*ViewChangeMsg `json:"setViewchangemsgs"` 	//V a set containing the valid ViewChageMsg 
	//SetPrepareMsgs map[int64]*PrepareMsg `json:"setPrepreparemsgs"`
	//PrepareMsg *PrepareMsg `json:"Preparemsg"`
	//O a set of PrePrepareMsgs from latest stable checkpoint(min-s) in V to the highest sequence number(max-s) in a PrepareMsg in V
	// new Primary creates a new PrePrepareMsg for view v+1 for each sequence number between min-s and max-s
	//Max_S int64 `json:"max_s"`
	Min_S int64 `json:"min_s"`
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
