package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/bigpicturelabs/consensusPBFT/pbft/network"
	"io/ioutil"
	"log"
	"os"
)

// Hard-coded for test.
var viewID = int64(10000000000)
var nodeTableForTest = []*network.NodeInfo {
	{NodeID: "Node1",  Url: "192.168.0.2:1111"},	//jaeyoung
	{NodeID: "Node2",     Url: "192.168.0.2:1112"},	//jaeyoung
	{NodeID: "Node3", Url: "192.168.0.25:1113"},	//yoomee
	{NodeID: "Node4",    Url: "192.168.0.25:1114"},	//yoomee
}

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

	if len(os.Args) < 2 {
		fmt.Println("Usage:", os.Args[0], "<nodeID> [node.list]")
		return
	}

	nodeID := os.Args[1]
	if len(os.Args) == 2 {
		fmt.Println("Node list are not specified")
		fmt.Println("Embedded list is used for test")
		nodeTable = nodeTableForTest
	} else {
		nodeListFile := os.Args[2]
		jsonFile, err := os.Open(nodeListFile)
		AssertError(err)
		defer jsonFile.Close()

		err = json.NewDecoder(jsonFile).Decode(&nodeTable)
		AssertError(err)
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

	server := network.NewServer(nodeID, nodeTable, viewID, decodePrivKey)

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
