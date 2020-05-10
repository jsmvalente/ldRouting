package bitcoindwrapper

type ListUnspentResult struct {
	TxID          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	Account       string  `json:"account"`
	ScriptPubKey  string  `json:"scriptPubKey"`
	RedeemScript  string  `json:"redeemScript,omitempty"`
	WitnessScript string  `json:"witnessScript"`
	Amount        float64 `json:"amount"`
	Confirmations int64   `json:"confirmations"`
	Spendable     bool    `json:"spendable"`
}

type GetBlockResult struct {
	// The block hash
	Hash string `json:"hash"`

	// The number of confirmations
	Confirmations uint64 `json:"confirmations"`

	// The block size
	Size uint64 `json:"size"`

	//The block size excluding witness data
	StrippedSize uint64 `json:"strippedsize"`

	//The block weight as defined in BIP 141
	Weight uint64 `json:"weight"`

	// The block height or index
	Height uint64 `json:"height"`

	// The block version
	Version uint32 `json:"version"`

	// The merkle root
	Merkleroot string `json:"merkleroot"`

	// Slice on transaction ids
	Tx []GetRawTransactionResult `json:"tx"`

	// The block time in seconds since epoch (Jan 1 1970 GMT)
	Time int64 `json:"time"`

	// The nonce
	Nonce uint64 `json:"nonce"`

	// The bits
	Bits string `json:"bits"`

	// The difficulty
	Difficulty float64 `json:"difficulty"`

	// Total amount of work in active chain, in hexadecimal
	Chainwork string `json:"chainwork,omitempty"`

	// The number of transactions in the block.
	NTx uint64 `json:"nTx"`

	// The hash of the previous block
	Previousblockhash string `json:"previousblockhash"`

	// The hash of the next block
	Nextblockhash string `json:"nextblockhash"`
}

type GetRawTransactionResult struct {
	InActiveChain bool   `json:"in_active_chain"`
	Hex           string `json:"hex"`
	Txid          string `json:"txid"`
	Version       uint32 `json:"version"`
	LockTime      uint32 `json:"locktime"`
	Vin           []Vin  `json:"vin"`
	Vout          []Vout `json:"vout"`
	BlockHash     string `json:"blockhash,omitempty"`
	Confirmations uint64 `json:"confirmations,omitempty"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime,omitempty"`
}

// A ScriptSig represents a scriptsyg
type ScriptSig struct {
	Asm string `json:"asm"`
	Hex string `json:"hex"`
}

// Vin represent an IN value
type Vin struct {
	Coinbase  string    `json:"coinbase"`
	Txid      string    `json:"txid"`
	Vout      int       `json:"vout"`
	ScriptSig ScriptSig `json:"scriptSig"`
	Sequence  uint32    `json:"sequence"`
}

type ScriptPubKey struct {
	Asm       string   `json:"asm"`
	Hex       string   `json:"hex"`
	ReqSigs   int      `json:"reqSigs,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
}

// Vout represent an OUT value
type Vout struct {
	Value        float64      `json:"value"`
	N            int          `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
}

type SignRawTransactionWithWalletResult struct {

	// The hex-encoded raw transaction with signature(s)
	Hex string `json:"hex"`
	// If the transaction has a complete set of signatures
	Complete bool `json:"complete"`
	//Script verification errors (if there are any)
	Errors []ScriptVerificationError `json:"errors"`
}

type ScriptVerificationError struct {

	// The hash of the referenced, previous transaction
	Txid string `json:"txid"`
	// The index of the output to spent and used as input
	Vout int `json:"vout"`
	// The hex-encoded signature script
	ScriptSig string `json:"scriptSig"`
	// Script sequence number
	Sequence int `json:"sequence"`
	// Verification or signing error related to the input
	Error string `json:"error"`
}
