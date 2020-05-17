package lndrlib

import (
	"fmt"
	"log"
	"strings"

	"github.com/jsmvalente/ldRouting/lndwrapper"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/tv42/zbase32"
)

//ConnectToLNClient connects to the local instance lnd
func ConnectToLNClient(host string, port int, macaroonPath string, tlsCertPath string) (*lndwrapper.Lnd, error) {

	lnd, err := lndwrapper.New(host, port, macaroonPath, tlsCertPath)
	if err != nil {
		log.Fatalln(err)
	}

	return lnd, nil
}

//GetLocalNodePubKey returns the lightning network id of the local node string
func GetLocalNodePubKey(client *lndwrapper.Lnd) [33]byte {

	nodeInfo, err := client.GetInfo()
	if err != nil {
		log.Fatalln(err)
	}

	return PubKeyStringToArray(nodeInfo.IdentityPubkey)
}

//GetNodeIPs - Get the IP of a certain node
func GetNodeIPs(client *lndwrapper.Lnd, nodePubKey [33]byte) []string {

	var addrs []string
	var i int
	nodePubKeyHexString := PubKeyArrayToString(nodePubKey)

	nodeInfo, err := client.GetNodeInfo(nodePubKeyHexString, false)
	if err != nil {
		log.Fatal(err)
	}

	for _, nodeAddress := range nodeInfo.Node.Addresses {
		i = strings.Index(nodeAddress.Addr, ":")
		addrs = append(addrs, nodeAddress.Addr[:i])
	}

	return addrs
}

//GetLocalNodeNeighboursPubKeys - Returns the pubkeys associated  of the the current node active neighbours
func GetLocalNodeNeighboursPubKeys(client *lndwrapper.Lnd) [][33]byte {

	neighboursPubKey := []string{}
	neighboursArrayPubKey := [][33]byte{}

	openChannels, err := client.ListChannels()
	if err != nil {
		log.Fatal(err)
	}

	for _, channel := range openChannels.GetChannels() {
		neighboursPubKey = append(neighboursPubKey, channel.RemotePubkey)
		neighboursArrayPubKey = append(neighboursArrayPubKey, PubKeyStringToArray(channel.RemotePubkey))
	}

	fmt.Println("Local neighbours: " + strings.Join(neighboursPubKey, ", "))

	return neighboursArrayPubKey
}

//GetLocalChannels - Returns the channels assocatited with the local node
func GetLocalChannels(client *lndwrapper.Lnd) []*lnrpc.Channel {

	openChannels, err := client.ListChannels()
	if err != nil {
		log.Fatal(err)
	}

	return openChannels.GetChannels()
}

//GetNodeNeighboursPubKeys - Returns the pubkeys associated  with neighbors of nodePubKey
func GetNodeNeighboursPubKeys(client *lndwrapper.Lnd, nodePubKey [33]byte) [][33]byte {

	neighboursPubKey := []string{}
	neighboursArrayPubKey := [][33]byte{}
	nodePubKeyHexString := PubKeyArrayToString(nodePubKey)

	nodeInfo, err := client.GetNodeInfo(nodePubKeyHexString, true)
	if err != nil {
		log.Fatal(err)
	}

	for _, channel := range nodeInfo.GetChannels() {
		neighboursPubKey = append(neighboursPubKey, channel.Node1Pub)
		neighboursArrayPubKey = append(neighboursArrayPubKey, PubKeyStringToArray(channel.Node1Pub))
	}

	fmt.Println("Neigbours of " + nodePubKeyHexString + ": " + strings.Join(neighboursPubKey, ", "))

	return neighboursArrayPubKey
}

//SignMessage signs a message using the keys provided by the lightning node
func SignMessage(client *lndwrapper.Lnd, message []byte) []byte {

	resp, err := client.SignMessage(message)
	if err != nil {
		log.Fatal(err)
	}

	//Get the corresponding signature bytes
	signature, err := zbase32.DecodeString(resp.Signature)
	if err != nil {
		log.Fatal(err)
	}

	return signature
}

//VerifyMessage verifies a message using the keys DB in the lightning node
func VerifyMessage(client *lndwrapper.Lnd, message []byte, signature []byte) (bool, [33]byte) {

	//Get the corresponding zbase32 signature
	zbase32Signature := zbase32.EncodeToString(signature)

	resp, err := client.VerifyMessage(message, zbase32Signature)
	if err != nil {
		log.Fatal(err)
	}

	return resp.Valid, PubKeyStringToArray(resp.Pubkey)
}
