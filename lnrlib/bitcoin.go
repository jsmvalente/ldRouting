package lndrlib

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"net"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/jsmvalente/ldRouting/bitcoindwrapper"
	"github.com/lightningnetwork/lnd/lnrpc"
)

const (

	//The satoshi value for the op return registration output
	registerOpRetOutAmount = 100
	//The satoshi value for the register tx fee
	registerTxFeeAmount = 1000
	//Version of the protocol we are in (hex)
	versionHex = "00000000"

	//Hex code for OP_RETURN and PUSHDATA+Data length
	opReturnHex = "6a"
	pushDataHex = "4c4c"

	//hex for the protocol identifier ASCII for "lar" (3 bytes = 0x6c6172)
	protocolIDHex = "6c6172"
)

type addressRegistration struct {
	address     [4]byte
	blockHeight uint64
	txID        [32]byte
	sig         [65]byte
	version     uint32
}

//ConnectToBitcoinClient connects to the local instance of the bitcoin-core client
func ConnectToBitcoinClient(host string, port int, rpcUser string, rpcPassword string) (*bitcoindwrapper.Bitcoind, error) {

	// Connect to local bitcoin core RPC server using HTTP POST mode.
	// Bitcoin core does not provide TLS by default
	// This won't hang if a bigcoin core client is not present in the machine since it just creates the rpc client
	bitcoind, err := bitcoindwrapper.New(host, port, rpcUser, rpcPassword, false)
	if err != nil {
		log.Fatalln(err)
	}

	return bitcoind, nil
}

//UnlockWallet Unlocks the wallet it to be used by other methods
func UnlockWallet(bitcoind *bitcoindwrapper.Bitcoind, passphrase string) error {
	err := bitcoind.WalletPassphrase(passphrase, 10)

	return err
}

//Gets the private key associated with a certain address
func privateKeyFromAddress(bitcoind *bitcoindwrapper.Bitcoind, address string) string {
	return ""
}

//BroadcastNewAddressTx broadcasts a new address regstration transaction into the blockchain
//Note: Requires bitcoin wallet to be unlocked
func BroadcastNewAddressTx(bitcoind *bitcoindwrapper.Bitcoind, lnClient lnrpc.LightningClient, address [4]byte) (string, error) {

	//TxID for the input transaction we are using
	var unsOutTxID string
	//Amount of coins in the unspent output
	var unsOutAmmount float64
	//Amount of coins in the unspent output (satoshis)
	var unsOutAmmountSats int64
	//Vout of the unspent output
	var unsOutVout uint32
	//Address of the unspent output
	var unsOutAddress string

	opRetOutAmount := int64(registerOpRetOutAmount)
	txFeeAmount := int64(registerTxFeeAmount)
	var changeAmount int64
	var registerTxHash string

	// Get the current block count.
	blockCount, err := bitcoind.GetBlockCount()
	if err != nil {
		return "", err
	}
	log.Printf("Block count: %d", blockCount)

	// Get the list of unspent transaction outputs (utxos) and choose one with some coin
	unsOuts, err := bitcoind.ListUnspent()
	if err != nil {
		log.Fatal(err)
		return "", err
	}
	log.Printf("Num unspent outputs (utxos): %d", len(unsOuts))

	//Find an unspent output with enough BTC to create a new transaction
	if len(unsOuts) > 0 {
		// Iterate through all the unspent outputs
		for n, unsOut := range unsOuts {

			unsOutTxID = unsOut.TxID
			unsOutVout = unsOut.Vout
			unsOutAmmount = unsOut.Amount
			unsOutAmmountSats = int64(unsOutAmmount * math.Pow(10, 8))
			unsOutAddress = unsOut.Address
			fmt.Printf("Found an UTXO with %f unspent BTC. TXHash is: %s\n", unsOutAmmount, unsOutTxID)

			// Found what we were looking for and can now calculate the change amount
			if unsOutAmmountSats > opRetOutAmount+txFeeAmount {
				changeAmount = unsOutAmmountSats - (opRetOutAmount + txFeeAmount)
				break
			}

			//If we get to the last unspent output witout finding one output
			if n == len(unsOuts) {
				return "", errors.New("No outputs with enough BTC to create registration tx")
			}
		}

		//Create the transaction we will broadcast
		tx := wire.NewMsgTx(wire.TxVersion)

		//Create the first output (OP_RETURN)
		opReturnScript, err := hex.DecodeString(opReturnHex + pushDataHex + protocolIDHex + versionHex)
		if err != nil {
			return "", err
		}
		opReturnScript = append(opReturnScript, address[:]...)
		signedAddress := SignMessage(lnClient, address[:])
		opReturnScript = append(opReturnScript, signedAddress...)

		txOutputReturn := wire.NewTxOut(opRetOutAmount, opReturnScript)
		tx.AddTxOut(txOutputReturn)

		//Create the second output with the change back to out address
		p2pkhAddress, err := btcutil.DecodeAddress(unsOutAddress, &(chaincfg.TestNet3Params))
		if err != nil {
			return "", err
		}
		p2pkhScript, err := txscript.PayToAddrScript(p2pkhAddress)
		if err != nil {
			return "", err
		}
		txOutputP2pkh := wire.NewTxOut(changeAmount, p2pkhScript)
		tx.AddTxOut(txOutputP2pkh)

		//Create the input and add it to the transaction
		txHash, err := chainhash.NewHashFromStr(unsOutTxID)
		if err != nil {
			return "", err
		}
		prevOut := wire.NewOutPoint(txHash, unsOutVout)
		txIn := wire.NewTxIn(prevOut, []byte{txscript.OP_0, txscript.OP_0}, nil)
		tx.AddTxIn(txIn)

		var buffer = new(bytes.Buffer)
		err = tx.Serialize(buffer)
		if err != nil {
			return "", err
		}
		serializedTx := hex.EncodeToString(buffer.Bytes())

		//Sign the Transaction
		signedTx, err := bitcoind.SignRawTransactionWithWallet(serializedTx)
		if err != nil {
			return "", err
		}

		//Send the transaction
		registerTxHash, err = bitcoind.SendRawTransaction(signedTx.Hex)
		if err != nil {
			return "", err
		}
	} else {
		return "", errors.New("No spendable outputs to create registration tx")
	}

	return registerTxHash, nil
}

//GetBlockCount gets the height og the best chain
func GetBlockCount(bitcoind *bitcoindwrapper.Bitcoind) (uint64, error) {
	blockCount, err := bitcoind.GetBlockCount()
	return blockCount, err
}

//Scans the blockchain for new Lighting addresses starting from a certain block and returns them
func getNewAddressRegistrations(bitcoind *bitcoindwrapper.Bitcoind, lnClient lnrpc.LightningClient, fromBlock uint64, toBlock uint64) []*addressRegistration {

	//Index of the block we are treating
	var blockIndex uint64
	//Hash of the bl0ock we are treating
	var blockHash string
	//Block we are treating
	var block bitcoindwrapper.GetBlockResult
	//Slice with all the addresses we Find
	var newAddressRegistrationList []*addressRegistration
	//variables to hold info on new addresses
	var newAddressRegistration *addressRegistration
	var txID [32]byte
	var newAddress [4]byte
	var version uint32
	var sig [65]byte
	var err error
	var txOuts []bitcoindwrapper.Vout
	var newAddressBytes, versionBytes, signatureBytes []byte

	//Search every output for Lighting addresses
	for blockIndex = fromBlock; blockIndex <= toBlock; blockIndex++ {

		//Print every 100 blocks
		if blockIndex%100 == 0 {
			log.Println("Height:", blockIndex)
		}

		//Get the hash of the block we are treating
		blockHash, err = bitcoind.GetBlockHash(blockIndex)
		if err != nil {
			log.Fatal(err)
		}

		//Get the block
		block, err = bitcoind.GetBlock(blockHash)
		if err != nil {
			log.Fatal(err)
		}

		//Search every transaction on this block
		for _, tx := range block.Tx {

			//Check if we are dealing with a valid registerNewAddress tx
			//Number of outputs should be 1 or 2
			//OP_RETURN output should have index 0
			//output number 1 is reserved for change
			//output script for output 0 should be:
			//OP_RETURN (1 byte = 0x6a) +
			//OP_PUSHDATA1 - The next byte contains the number of bytes to be pushed onto the stack (1 byte = 0x4c) +
			//Data size is 0x4c=76 bytes (1 byte = 0x4c)
			//ASCII for "lar" (3 bytes = 0x6c6172) +
			//protocol version (4 bytes)
			//lighting address to be registered (4 bytes) +
			//signature of lighting address signed by the node pub key (65 bytes)

			txOuts = tx.Vout

			//Check if this transaction output set the format we are looking
			if (len(txOuts) == 2) &&
				len(txOuts[0].ScriptPubKey.Hex) == 158 &&
				txOuts[0].ScriptPubKey.Hex[:12] == "6a4c4c6c6172" {

				//Get the tx hash of the registering tx
				txIDByteString, err := hex.DecodeString(tx.Txid)
				if err != nil {
					log.Fatal(err)
				}
				copy(txID[:], txIDByteString)

				//Get the protocol version
				versionBytes, err = hex.DecodeString(txOuts[0].ScriptPubKey.Hex[12:20])
				if err != nil {
					log.Fatal(err)
				}
				version = binary.BigEndian.Uint32(versionBytes)

				//Get the lighting address to be registered
				newAddressBytes, err = hex.DecodeString(txOuts[0].ScriptPubKey.Hex[20:28])
				if err != nil {
					log.Fatal(err)
				}
				copy(newAddress[:], newAddressBytes)

				//Get the signature of lighting address signed by the node that registered it
				signatureBytes, err = hex.DecodeString(txOuts[0].ScriptPubKey.Hex[28:])
				if err != nil {
					log.Fatal(err)
				}
				copy(sig[:], signatureBytes)

				newAddressRegistration = &(addressRegistration{address: newAddress, blockHeight: blockIndex, txID: txID, sig: sig, version: version})
				newAddressRegistrationList = append(newAddressRegistrationList, newAddressRegistration)

				log.Printf("Found registration for address '%s' in block %d\n", net.IP(newAddressBytes).String(), blockIndex)
			}
		}
	}

	log.Printf("Done iterating through blocks.")

	return newAddressRegistrationList
}
