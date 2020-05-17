package lndwrapper

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

// An Lnd represents an lnd client
type Lnd struct {
	client lnrpc.LightningClient
}

//GetInfoResponse is an alias for the wrapped lnrpc type
type GetInfoResponse = lnrpc.GetInfoResponse

//NodeInfo is an alias for the wrapped lnrpc type
type NodeInfo = lnrpc.NodeInfo

//ListChannelsResponse is an alias for the wrapped lnrpc type
type ListChannelsResponse = lnrpc.ListChannelsResponse

//SignMessageResponse is an alias for the wrapped lnrpc type
type SignMessageResponse = lnrpc.SignMessageResponse

//VerifyMessageResponse is an alias for the wrapped lnrpc type
type VerifyMessageResponse = lnrpc.VerifyMessageResponse

// New return a new lnd
func New(host string, port int, macaroonPath string, tlsCertPath string) (*Lnd, error) {

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

	return &Lnd{lnrpc.NewLightningClient(conn)}, nil
}

//GetInfo returns some info about the node
func (lnd *Lnd) GetInfo() (*GetInfoResponse, error) {

	ctxb := context.Background()
	req := &lnrpc.GetInfoRequest{}
	info, err := lnd.client.GetInfo(ctxb, req)
	if err != nil {
		return nil, err
	}
	return info, nil
}

//GetNodeInfo returns some info about a node identified by pubkey
func (lnd *Lnd) GetNodeInfo(pubkey string, includeChannels bool) (*NodeInfo, error) {

	ctxb := context.Background()
	req := &lnrpc.NodeInfoRequest{
		PubKey:          pubkey,
		IncludeChannels: includeChannels,
	}
	info, err := lnd.client.GetNodeInfo(ctxb, req)
	if err != nil {
		return nil, err
	}

	return info, nil
}

//ListChannels returns a list of active channels
func (lnd *Lnd) ListChannels() (*ListChannelsResponse, error) {
	ctxb := context.Background()
	req := &lnrpc.ListChannelsRequest{}

	channels, err := lnd.client.ListChannels(ctxb, req)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// SignMessage signs a message with this node's private key.
//The returned 65 byte signature string is zbase32 encoded and pubkey recoverable,
//meaning that only the message digest and signature are needed for verification.
func (lnd *Lnd) SignMessage(message []byte) (*SignMessageResponse, error) {

	ctxb := context.Background()
	req := &lnrpc.SignMessageRequest{Msg: message}

	resp, err := lnd.client.SignMessage(ctxb, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// VerifyMessage verifies a 65 byte signature over a msg enconded as a zbase32 string.
//The signature must be signed by an active node in the resident node's channel database.
//In addition to returning the validity of the signature,
//VerifyMessage also returns the recovered pubkey from the signature.
func (lnd *Lnd) VerifyMessage(message []byte, signature string) (*VerifyMessageResponse, error) {

	ctxb := context.Background()
	req := &lnrpc.VerifyMessageRequest{Msg: message, Signature: signature}

	resp, err := lnd.client.VerifyMessage(ctxb, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
