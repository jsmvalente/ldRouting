package lndrlib

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/tv42/zbase32"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	macaroon "gopkg.in/macaroon.v2"
)

//ConnectToLNClient connects to the local instance lnd
func ConnectToLNClient(host string, port int, macaroonPath string, tlsCertPath string) (lnrpc.LightningClient, error) {

	tlsCreds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
	if err != nil {
		fmt.Println("Cannot get node tls credentials", err)
		return nil, err
	}

	macaroonBytes, err := ioutil.ReadFile(macaroonPath)
	if err != nil {
		fmt.Println("Cannot read macaroon file", err)
		return nil, err
	}

	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macaroonBytes); err != nil {
		fmt.Println("Cannot unmarshal macaroon", err)
		return nil, err
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(macaroons.NewMacaroonCredential(mac)),
	}

	target := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.Dial(target, opts...)
	if err != nil {
		fmt.Println("Cannot dial to lnd", err)
		return nil, err
	}

	return lnrpc.NewLightningClient(conn), nil
}

//GetLocalNodePubKey returns the lightning network id of the local node string
func GetLocalNodePubKey(client lnrpc.LightningClient) [33]byte {

	ctxb := context.Background()
	req := &lnrpc.GetInfoRequest{}
	info, err := client.GetInfo(ctxb, req)
	if err != nil {
		log.Fatal(err)
	}

	return PubKeyStringToArray(info.IdentityPubkey)
}

//GetNodeIPs - Get the IP of a certain node
func GetNodeIPs(client lnrpc.LightningClient, nodePubKey [33]byte) []string {

	var addrs []string
	var i int
	nodePubKeyHexString := PubKeyArrayToString(nodePubKey)

	ctxb := context.Background()
	req := &lnrpc.NodeInfoRequest{
		PubKey:          nodePubKeyHexString,
		IncludeChannels: false,
	}
	nodeInfo, err := client.GetNodeInfo(ctxb, req)
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
func GetLocalNodeNeighboursPubKeys(client lnrpc.LightningClient) [][33]byte {

	neighboursPubKey := []string{}
	neighboursArrayPubKey := [][33]byte{}
	ctxb := context.Background()
	req := &lnrpc.ListChannelsRequest{}

	openChannels, err := client.ListChannels(ctxb, req)
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
func GetLocalChannels(client lnrpc.LightningClient) []*lnrpc.Channel {

	ctxb := context.Background()
	req := &lnrpc.ListChannelsRequest{}

	openChannels, err := client.ListChannels(ctxb, req)
	if err != nil {
		log.Fatal(err)
	}

	return openChannels.GetChannels()
}

//GetNodeNeighboursPubKeys - Returns the pubkeys associated  with neighbors of nodePubKey
func GetNodeNeighboursPubKeys(client lnrpc.LightningClient, nodePubKey [33]byte) [][33]byte {

	neighboursPubKey := []string{}
	neighboursArrayPubKey := [][33]byte{}
	nodePubKeyHexString := PubKeyArrayToString(nodePubKey)

	ctxb := context.Background()
	req := &lnrpc.NodeInfoRequest{
		PubKey:          nodePubKeyHexString,
		IncludeChannels: true,
	}

	nodeInfo, err := client.GetNodeInfo(ctxb, req)
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

// SignMessage signs a message with this node's private key.
//The returned 65 byte signature string is zbase32 encoded and pubkey recoverable,
//meaning that only the message digest and signature are needed for verification.
func SignMessage(client lnrpc.LightningClient, message []byte) []byte {

	ctxb := context.Background()
	req := &lnrpc.SignMessageRequest{Msg: message}

	resp, err := client.SignMessage(ctxb, req)
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

// VerifyMessage verifies a 65 byte signature over a msg.
//The signature must be signed by an active node in the resident node's channel database.
//In addition to returning the validity of the signature,
//VerifyMessage also returns the recovered pubkey from the signature.
func VerifyMessage(client lnrpc.LightningClient, message []byte, signature []byte) (bool, [33]byte) {

	//Get the corresponding zbase32 signature
	zbase32Signature := zbase32.EncodeToString(signature)

	ctxb := context.Background()
	req := &lnrpc.VerifyMessageRequest{Msg: message, Signature: zbase32Signature}

	resp, err := client.VerifyMessage(ctxb, req)
	if err != nil {
		log.Fatalln(err)
	}

	return resp.Valid, PubKeyStringToArray(resp.Pubkey)
}
