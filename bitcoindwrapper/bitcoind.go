package bitcoindwrapper

import (
	"encoding/json"
	"log"
	"strconv"
)

const (
	// VERSION represents bicoind package version
	version = 0.1
	// DEFAULT_RPCCLIENT_TIMEOUT represent http timeout for rcp client
	rpcclientTimeout = 30
)

// A Bitcoind represents a Bitcoind client
type Bitcoind struct {
	client *rpcClient
}

// New return a new bitcoind
func New(host string, port int, user, passwd string, useSSL bool, timeoutParam ...int) (*Bitcoind, error) {
	var timeout int = rpcclientTimeout
	// If the timeout is specified in timeoutParam, allow it.
	if len(timeoutParam) != 0 {
		timeout = timeoutParam[0]
	}

	rpcClient, err := newClient(host, port, user, passwd, useSSL, timeout)
	if err != nil {
		return nil, err
	}
	return &Bitcoind{rpcClient}, nil
}

// GetBlockCount returns the number of blocks in the longest block chain.
func (b *Bitcoind) GetBlockCount() (uint64, error) {
	log.Println("Calling 'getblockcount'")
	r, err := b.client.call("getblockcount", nil)
	if err = handleError(err, &r); err != nil {
		return 0, err
	}

	count, err := strconv.ParseUint(string(r.Result), 10, 64)

	return count, err
}

// ListUnspent returns array of unspent transaction inputs in the wallet.
func (b *Bitcoind) ListUnspent() ([]ListUnspentResult, error) {
	log.Println("Calling 'listunspent'")
	r, err := b.client.call("listunspent", nil)
	if err = handleError(err, &r); err != nil {
		return nil, err
	}
	result := []ListUnspentResult{}
	err = json.Unmarshal(r.Result, &result)

	return result, err
}

// GetBlockHash returns hash of block in best-block-chain at <index>
func (b *Bitcoind) GetBlockHash(index uint64) (string, error) {
	var hash string
	r, err := b.client.call("getblockhash", []uint64{index})
	if err = handleError(err, &r); err != nil {
		return "", err
	}
	err = json.Unmarshal(r.Result, &hash)

	return hash, err
}

// SignRawTransactionWithWallet RPC sign inputs for raw transaction (serialized, hex-encoded).
func (b *Bitcoind) SignRawTransactionWithWallet(rawTx string) (*SignRawTransactionWithWalletResult, error) {
	log.Println("Calling 'signrawtransactionwithwallet'")
	r, err := b.client.call("signrawtransactionwithwallet", []string{rawTx})
	if err = handleError(err, &r); err != nil {
		return nil, err
	}

	result := &SignRawTransactionWithWalletResult{}
	err = json.Unmarshal(r.Result, result)

	return result, err
}

// SendRawTransaction RPC submits raw transaction (serialized, hex-encoded) to local node and network.
func (b *Bitcoind) SendRawTransaction(signedTx string) (string, error) {
	var txHash string
	log.Println("Calling 'sendrawtransaction'")
	r, err := b.client.call("sendrawtransaction", []string{signedTx})
	if err = handleError(err, &r); err != nil {
		return "", err
	}
	err = json.Unmarshal(r.Result, &txHash)

	return txHash, err
}

// GetBlock returns information about the block with the given hash.
func (b *Bitcoind) GetBlock(blockHash string) (*GetBlockResult, error) {
	r, err := b.client.call("getblock", []interface{}{blockHash, 2})
	if err = handleError(err, &r); err != nil {
		return nil, err
	}
	result := &GetBlockResult{}
	err = json.Unmarshal(r.Result, &result)

	return result, err
}

// WalletPassphrase stores the wallet decryption key in memory for <timeout> seconds.
func (b *Bitcoind) WalletPassphrase(passPhrase string, timeout uint64) error {
	log.Println("Calling 'walletpassphrase'")
	r, err := b.client.call("walletpassphrase", []interface{}{passPhrase, timeout})

	return handleError(err, &r)
}
