package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/bigpicturelabs/consensusPBFT/pbft/network"
	"io/ioutil"
	"log"
	"os"
	"strconv"
)

// Hard-coded for test.
var viewID = int64(10000000000)
//var nodeTableForTest = []*network.NodeInfo {
//	{NodeID: "Node1",  Url: "localhost:1111"},
//	{NodeID: "Node2",     Url: "localhost:1112"},
//	{NodeID: "Node3", Url: "localhost:1113"},
//	{NodeID: "Node4",    Url: "localhost:1114"},
//
//	{NodeID: "Node1",  Url: "192.168.0.2:1111"},	//jaeyoung
//	{NodeID: "Node2",     Url: "192.168.0.2:1112"},	//jaeyoung
//	{NodeID: "Node3", Url: "192.168.0.25:1113"},	//yoomee
//	{NodeID: "Node4",    Url: "192.168.0.25:1114"},	//yoomee
//}

func PrivateKeyDecode(pemEncoded []byte) *ecdsa.PrivateKey {
	blockPriv, _ := pem.Decode(pemEncoded)
	x509Encoded := blockPriv.Bytes
	privateKey, _ := x509.ParseECPrivateKey(x509Encoded)

	return privateKey
}

func PublicKeyDecode(pemEncoded []byte) *ecdsa.PublicKey {
	blockPub, _ := pem.Decode(pemEncoded)
	x509EncodedPub := blockPub.Bytes
	genericPublicKey, _ := x509.ParsePKIXPublicKey(x509EncodedPub)
	publicKey := genericPublicKey.(*ecdsa.PublicKey)

	return publicKey
}

func main() {
	var nodeTable []*network.NodeInfo
	var seedNodeTables [20][]*network.NodeInfo
	if len(os.Args) < 2 {
		fmt.Println("Usage:", os.Args[0], "<nodeID> [node.list]")
		return
	}
	nodeID := os.Args[1]

	var seedListFile [20]string
	var nodeListFile=""
	if len(os.Args) == 3{
		for i := 1; i <20; i++ {
			seedListFile[i] = "./seedList/nodeNum"+os.Args[2]+"/seedList"+strconv.Itoa(i)+".txt"
		}
		nodeListFile = "./seedList/nodeNum"+os.Args[2]+"/nodeList.json"
	} else if len(os.Args) == 4{
		if os.Args[3] != "AWS" {
			for i := 1; i < 20; i++ {
				seedListFile[i] = "./seedList/nodeNum" + os.Args[2] + "/seedList" + strconv.Itoa(i) + ".txt"
			}
			nodeListFile = "./seedList/nodeNum" + os.Args[2] + "/nodeList2_" + os.Args[2] + ".json"
		} else {
			for i := 1; i < 20; i++ {
				seedListFile[i] = "./seedList/nodeNum" + os.Args[2] + "/seedList" + strconv.Itoa(i) + ".txt"
			}
			nodeListFile = "./seedList/nodeNum" + os.Args[2] + "/nodeList_aws.json"
		}
	}
	jsonFile, err := os.Open(nodeListFile)
	AssertError(err)
	defer jsonFile.Close()
	err = json.NewDecoder(jsonFile).Decode(&nodeTable)
	AssertError(err)

	for i :=1; i<20; i++ {
		var tmp []*network.NodeInfo
		file, err := os.Open(seedListFile[i])
		AssertError(err)
		scanner:= bufio.NewScanner(file)
		for scanner.Scan(){
			s := scanner.Text()
			for _, nodeInfo := range nodeTable {
				if nodeInfo.NodeID == s {
					tmp = append(tmp,nodeInfo)
					break
				}
			}
		}
		file.Close()
		seedNodeTables[i] = tmp
	}
	for i:=1; i<20; i++ {
		for j:=0; j<len(nodeTable); j++{
			var n *network.NodeInfo = seedNodeTables[i][j]
			fmt.Printf("%s ",n.NodeID)
		}
		fmt.Println()
	}
	// Load public key for each node.
	for _, nodeInfo := range nodeTable {
		pubKeyFile := fmt.Sprintf("keys/%s.pub", nodeInfo.NodeID)
		pubBytes, err := ioutil.ReadFile(pubKeyFile)
		AssertError(err)

		decodePubKey := PublicKeyDecode(pubBytes)
		nodeInfo.PubKey = decodePubKey
	}
	// Make NodeID PriveKey
	privKeyFile := fmt.Sprintf("keys/%s.priv", nodeID)
	privbytes, err := ioutil.ReadFile(privKeyFile)
	AssertError(err)
	decodePrivKey := PrivateKeyDecode(privbytes)

	server := network.NewServer(nodeID, nodeTable, seedNodeTables, viewID, decodePrivKey)

	if server != nil {
		server.Start()
	}
}

func AssertError(err error) {
	if err == nil {
		return
	}

	log.Println(err)
	os.Exit(1)
}
