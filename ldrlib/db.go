package ldrlib

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/jsmvalente/ldRouting/bitcoindwrapper"
	"github.com/jsmvalente/ldRouting/lndwrapper"
	"github.com/lightningnetwork/lnd/lnrpc"
)

const (
	addressInfoSerializedSize  int    = 81
	routingEntrySerializedSize int    = 24
	blockHeightSerializedSize  int    = 8
	genesisBlock               uint64 = 0
	addressDBFileName          string = "address.db"
	routingDBFileName          string = "routing.db"
)

//AddressInfo is used to store in memory the info associated with an registered address
//address: the routing address
//NodePubKey:  the 33 byte compressed pubkey of the registering node
//registrationHeight: the height of the block where the address was registered
// registrationTxID: the tx id (hash of the transaction) where the address was registered
//version: version of the protocol in which this block was registered
//routingEntries: the routing entries associated with this address, the destionation field
//of the routing entries should be equal to address
type addressInfo struct {
	address            [4]byte
	nodePubKey         [33]byte
	registrationHeight uint64
	registrationTxID   [32]byte
	version            uint32
	routingEntry       *routingEntry
	peerConn           *connInfo
}

//routingEntry holds a routing entry that is to be associated with an address destination and stored locally
//destination: the destination node's address
//hop: the next hop's address
//capacity: the known minimum capacity for this route
//height: block height in which the entry was updated
type routingEntry struct {
	destination [4]byte
	nextHop     [4]byte
	capacity    int64
	height      uint64
}

//DB type, the head node of the database tree and the height of the last scanned block
type DB struct {
	filePath            string
	height              uint64
	addressTreeHead     *node
	keyToAddressMap     map[[33]byte]*node
	routingEntriesStack *routingStack
	localAddress        [4]byte
	destConns           map[string]*connInfo
}

func createDB(dbPath string) *DB {

	var binaryTree = createBinaryTree()
	var stringByteMap = make(map[[33]byte]*node)
	var destConnMap = make(map[string]*connInfo)

	db := DB{filePath: dbPath, height: genesisBlock,
		addressTreeHead: binaryTree, keyToAddressMap: stringByteMap,
		routingEntriesStack: createRoutingStack(), destConns: destConnMap}

	return &db
}

//ReadDBFromDisk reads the address database file and stores it in memory returning the DB
//Address Database follows the following rules:
//<blockHeight> (8 bytes)
// n * <addressInfo> n * (81 bytes)
//<addressInfo>:
//<address> (4 bytes) + <nodePubKey> (33 bytes) + 8 (registrationHeight) +  32 (registrationTxID) + 4 (version)
//Routing Database follows the following rules:
//m * <routingEntry>
//<routingEntry>:
//<destination> (4 bytes) + <hop> (4 bytes) + <capacity>  (8 bytes) + <height>  (8 bytes)
func ReadDBFromDisk(dataPath string, lnClient *lndwrapper.Lnd) *DB {

	//Create the local database
	var addressInfoBytes = make([]byte, addressInfoSerializedSize)
	var addressInfo *addressInfo
	var blockHeightBytes = make([]byte, blockHeightSerializedSize)
	var blockHeight uint64
	var routingEntryBytes = make([]byte, routingEntrySerializedSize)
	var routingEntry *routingEntry
	var addressDBPath = path.Join(dataPath, addressDBFileName)
	var routingDBPath = path.Join(dataPath, routingDBFileName)
	var db = createDB(dataPath)

	// Create the path where the data is to be stored (if it doesnt exist)
	os.MkdirAll(dataPath, 0755)

	//Open the address database file
	log.Println("Opening Address DB file.")
	addressfp, err := os.OpenFile(addressDBPath, os.O_CREATE|os.O_RDONLY, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer addressfp.Close()

	fileStat, err := addressfp.Stat()
	if err != nil {
		log.Fatal(err)
	}

	//If the file is empty we write the genesis routing block into it to kickstart the address DB
	if fileStat.Size() == 0 {
		log.Println("Found empty address DB")

		db.updateBlockHeight(genesisBlock)
	} else {
		// Read the last synced block of the database
		log.Println("Reading DB block height")
		n, err := addressfp.Read(blockHeightBytes)
		log.Println("Read", n, "bytes from address DB file.")
		//File is empty
		if err == io.EOF {
			log.Println("Reached end of file")
			return db
		}
		if err != nil {
			log.Fatal("Read failed:", err)
		}

		log.Println("Updating block height")
		blockHeight = deserializeBlockHeight(blockHeightBytes)
		db.updateBlockHeight(blockHeight)

		for {
			// Read an address and its associated data
			log.Println("Reading new address")
			n, err := addressfp.Read(addressInfoBytes)
			log.Println("Read", n, "bytes from address DB file.")
			//Reached end of file
			if err == io.EOF {
				log.Println("Reached end of file")
				break
			}
			if err != nil {
				log.Fatal(err)
			}

			//Add address to dheadatabase
			addressInfo = deserializeAddressInfo(addressInfoBytes)
			db.addAddressToDB(addressInfo)
		}
	}

	//Open the routing database file
	log.Println("Opening Routing DB file.")
	routingfp, err := os.OpenFile(routingDBPath, os.O_CREATE|os.O_RDONLY, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer routingfp.Close()

	fileStat, err = routingfp.Stat()
	if err != nil {
		log.Fatal(err)
	}

	//If the file is empty we add our own peers (direct destinations)
	if fileStat.Size() == 0 {
		log.Println("Found empty routing DB")
	} else {
		//Otherwise we Read every routing entry for every address
		for {
			// Read a routing entry
			log.Println("Reading new routing entry")
			n, err := routingfp.Read(routingEntryBytes)
			log.Println("Read", n, "bytes from routing DB file.")
			//Reached end of file
			if err == io.EOF {
				log.Println("Reached end of file")
				break
			}
			if err != nil {
				log.Fatal(err)
			}

			//Add routing entry to the DB
			routingEntry = deserializeRoutingEntry(routingEntryBytes)
			db.addRoutingEntryToDB(routingEntry)
		}
	}

	return db
}

//GetBlockHeight returns the height of the last synced block
func (db *DB) getBlockHeight() uint64 {
	return db.height
}

//SaveLocalAddress saves the local address to the DB
func (db *DB) SaveLocalAddress(localAddress [4]byte) {
	copy(db.localAddress[:], localAddress[:])
}

func (db *DB) getLocalAddress() [4]byte {
	return db.localAddress
}

//updateBlockHeight updates the block height by updating memory and disk
func (db *DB) updateBlockHeight(blockHeight uint64) {

	var addressDBPath = path.Join(db.filePath, addressDBFileName)

	//Update memory
	db.height = blockHeight
	blockHeightBytes := serializeBlockHeight(blockHeight)

	//Store the blockHeight in disk
	// It will be created if it doesn't exist and close at the end of the function
	fp, err := os.OpenFile(addressDBPath, os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer fp.Close()

	n, err := fp.Write(blockHeightBytes)
	log.Println("Wrote", n, "bytes to DB file.")
	if err != nil {
		log.Fatalln("Write failed:", err)
	}

	log.Println("DB Block Height updated to " + strconv.FormatUint(blockHeight, 10))
}

//GetNodeAddress retruns the routing address of the node associated with
//the lightning pubkey id
func (db *DB) GetNodeAddress(pubkey [33]byte) ([4]byte, bool) {

	addressNode, isPresent := db.keyToAddressMap[pubkey]

	if !isPresent {
		return [4]byte{}, false
	}

	addressInfo := addressNode.getData().(*addressInfo)
	return addressInfo.address, true
}

//GetAddressNode returns the public key associated with a certain LDR address
func (db *DB) GetAddressNode(address [4]byte) [33]byte {
	// Get the head of the binaryTree
	var head = db.addressTreeHead
	//Get the address so we know the path in the binaryTree
	var bitAddress = byteToBit(address)

	for i := 0; i < len(bitAddress); i++ {
		if bitAddress[i] {
			head = head.rightChild()
			if head == nil {
				return [33]byte{}
			}
		} else {
			head = head.leftChild()
			if head == nil {
				return [33]byte{}
			}
		}
	}

	return (head.getData().(*addressInfo)).nodePubKey
}

//IsNodeRegistered checks if the node idenfied by pubkey is registered
//in the routing protocol
func (db *DB) IsNodeRegistered(pubkey [33]byte) bool {

	_, isPresent := db.keyToAddressMap[pubkey]

	if isPresent {
		return true
	}

	return false
}

//IsAddressRegistered checks if an address is registered
func (db *DB) IsAddressRegistered(address [4]byte) bool {

	// Get the head of the binaryTree
	var head = db.addressTreeHead
	//Get the address so we know the path in the binaryTree
	var bitAddress = byteToBit(address)

	for i := 0; i < len(bitAddress); i++ {
		if bitAddress[i] {
			head = head.rightChild()
			if head == nil {
				return false
			}
		} else {
			head = head.leftChild()
			if head == nil {
				return false
			}
		}
	}

	log.Println("Address", address, "is registered in the DB.")
	return true
}

//SuggestAddress suggests an address from a neighbour Address
func (db *DB) SuggestAddress(seedAddress [4]byte) ([4]byte, error) {

	head := db.addressTreeHead

	//Get the bit neighbour address so we know the path in the binaryTree
	var bitSeedAddress = byteToBit(seedAddress)

	foundFlag, suggestedAddress := closestFreeAddress(head, bitSeedAddress, 0)
	if foundFlag {
		return suggestedAddress, nil
	}

	return [4]byte{}, errors.New("Can't suggest address for " + net.IP(seedAddress[:]).String())
}

func closestFreeAddress(head *node, bitSeedAddress [32]bool, positionIndex int) (bool, [4]byte) {

	//We are at the leave
	if head.leftChild() == nil && head.rightChild() == nil {
		return false, [4]byte{}
	}

	if bitSeedAddress[positionIndex] {
		head = head.rightChild()
	} else {
		head = head.leftChild()
	}

	foundFlag, suggestedAddress := closestFreeAddress(head, bitSeedAddress, positionIndex+1)
	if foundFlag {
		return foundFlag, suggestedAddress
	}

	foundFlag, suggestedAddress = bottomSearch(head, bitSeedAddress, positionIndex+1)
	if foundFlag {
		log.Println(foundFlag, suggestedAddress, positionIndex)
		return foundFlag, suggestedAddress
	}

	return false, [4]byte{}
}

func bottomSearch(head *node, bitSeedAddress [32]bool, pathPosition int) (bool, [4]byte) {

	//Can't do bottom search on leaf nodes
	if head.leftChild() == nil && head.rightChild() == nil {
		return false, [4]byte{}
	}

	if bitSeedAddress[pathPosition] {

		head = head.leftChild()

		if head == nil {
			//Get a copy of the seed address so we can change it
			sugestedAddress := bitSeedAddress
			sugestedAddress[pathPosition] = false
			return true, bitToByte(rightPadBitAddress(sugestedAddress, pathPosition+1, true))
		}

		foundFlag, suggestedAddressBits := rightToLeftDFS(head, bitSeedAddress, pathPosition)
		return foundFlag, bitToByte(suggestedAddressBits)

	}

	head = head.rightChild()

	if head == nil {
		//Get a copy of the seed address so we can change it
		suggestedAddress := bitSeedAddress
		suggestedAddress[pathPosition] = true
		return true, bitToByte(rightPadBitAddress(suggestedAddress, pathPosition+1, false))
	}
	foundFlag, suggestedAddressBits := leftToRightDFS(head, bitSeedAddress, pathPosition)
	return foundFlag, bitToByte(suggestedAddressBits)

}

//Right pads the bitAddress starting at position startPosition with padBit
func rightPadBitAddress(bitAddress [32]bool, startPosition int, padBit bool) [32]bool {

	for i := startPosition; i < len(bitAddress); i++ {
		bitAddress[i] = padBit
	}

	return bitAddress
}

//Implements a left to right DFS, stopping when it finds a new address
func leftToRightDFS(head *node, bitNeighbourAddress [32]bool, pathPosition int) (bool, [32]bool) {

	if head == nil {
		return true, rightPadBitAddress(bitNeighbourAddress, pathPosition, false)
	}

	//Got to the leaf
	if head.leftChild() == nil && head.rightChild() == nil {
		return false, [32]bool{}
	}

	leftHead := head.leftChild()

	foundFlag, suggestedAddressBits := leftToRightDFS(leftHead, bitNeighbourAddress, pathPosition+1)
	if foundFlag {
		return foundFlag, suggestedAddressBits
	}

	rightHead := head.rightChild()

	foundFlag, suggestedAddressBits = leftToRightDFS(rightHead, bitNeighbourAddress, pathPosition+1)
	if foundFlag {
		return foundFlag, suggestedAddressBits
	}

	return false, [32]bool{}

}

func rightToLeftDFS(head *node, bitNeighbourAddress [32]bool, pathPosition int) (bool, [32]bool) {

	if head == nil {
		return true, rightPadBitAddress(bitNeighbourAddress, pathPosition, false)
	}

	//Got to the leaf
	if head.leftChild() == nil && head.rightChild() == nil {
		return false, [32]bool{}
	}

	rightHead := head.rightChild()

	foundFlag, suggestedAddressBits := rightToLeftDFS(rightHead, bitNeighbourAddress, pathPosition+1)
	if foundFlag {
		return foundFlag, suggestedAddressBits
	}

	leftHead := head.leftChild()

	foundFlag, suggestedAddressBits = rightToLeftDFS(leftHead, bitNeighbourAddress, pathPosition+1)
	if foundFlag {
		return foundFlag, suggestedAddressBits
	}

	return false, [32]bool{}
}

//loads the address into memory
func (db *DB) addAddressToDB(info *addressInfo) {
	// Get the head of the binaryTree
	var head = db.addressTreeHead
	var descendent *node
	//Get the address so we know the path in the binaryTree
	var bitAddress = byteToBit(info.address)

	for i := 0; i < len(bitAddress); i++ {
		if bitAddress[i] {
			descendent = head.rightChild()
			//Create the right child if it doesn't exist
			if descendent == nil {
				head = head.createRightChild()
			} else {
				head = descendent
			}
		} else {
			descendent = head.leftChild()
			//Create the left child if it doesn't exist
			if descendent == nil {
				head = head.createLeftChild()
			} else {
				head = descendent
			}
		}
	}

	//Save the address info into the tree node that represents the address
	head.saveData(info)

	//NodePubKey is a key to the map where we store pointers to the head where we previously stored the info
	db.keyToAddressMap[info.nodePubKey] = head

	log.Println("Address " + net.IP(info.address[:]).String() + " loaded to DB")
}

//saveAddressToFile adds a new address to database file
func (db *DB) saveAddressToFile(info *addressInfo) {

	var addressDBPath = path.Join(db.filePath, addressDBFileName)

	//Store the addresss in disk
	fp, err := os.OpenFile(addressDBPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer fp.Close()

	//SErialization of the address info
	addressInfoBytes := serializeAddressInfo(info)

	//Write serialization to the file
	n, err := fp.Write(addressInfoBytes)
	log.Println("Appended", n, "bytes to DB file.")
	if err != nil {
		log.Fatalln("Write failed:", err)
	}

	log.Println("Address " + net.IP(info.address[:]).String() + " added to DB")
}

func (db *DB) getRoutingEntry(destination [4]byte) *routingEntry {

	// Get the head of the binaryTree
	var head = db.addressTreeHead
	//Get the address so we know the path in the binaryTree
	var bitAddress = byteToBit(destination)

	for i := 0; i < len(bitAddress); i++ {
		if bitAddress[i] {
			head = head.rightChild()
		} else {
			head = head.leftChild()
		}
	}

	return (head.getData().(*addressInfo)).routingEntry
}

//loads a routing entry into the DB, replacing the existing one, if it exists
func (db *DB) addRoutingEntryToDB(entry *routingEntry) {

	// Get the head of the binaryTree
	var head = db.addressTreeHead
	//Get the address so we know the path in the binaryTree
	var bitAddress = byteToBit(entry.destination)

	for i := 0; i < len(bitAddress); i++ {
		if bitAddress[i] {
			head = head.rightChild()
		} else {
			head = head.leftChild()
		}
	}

	addressInfo := (head.getData().(*addressInfo))
	currentEntry := addressInfo.routingEntry
	//If there's already a routing entry for this destination we need to delete it from the stack
	if currentEntry != nil {
		//Delete routing entry from stack
		db.routingEntriesStack.remove(currentEntry)
	}

	//Add entry to the tree
	addressInfo.routingEntry = entry

	//Add the entry to the stack
	db.routingEntriesStack.put(entry)

	log.Println("Added routing entry to DB:", entry)
}

func (db *DB) getPeerConn(destination [4]byte) *connInfo {

	// Get the head of the binaryTree
	var head = db.addressTreeHead
	//Get the address so we know the path in the binaryTree
	var bitAddress = byteToBit(destination)

	for i := 0; i < len(bitAddress); i++ {
		if bitAddress[i] {
			head = head.rightChild()
		} else {
			head = head.leftChild()
		}
	}

	return (head.getData().(*addressInfo)).peerConn
}

//loads a peer connection into the DB, replacing the existing one, if it exists
func (db *DB) addPeerConnToDB(address [4]byte, peerConn *connInfo) {

	// Get the head of the binaryTree
	var head = db.addressTreeHead
	//Get the address so we know the path in the binaryTree
	var bitAddress = byteToBit(address)

	for i := 0; i < len(bitAddress); i++ {
		if bitAddress[i] {
			head = head.rightChild()
		} else {
			head = head.leftChild()
		}
	}

	addressInfo := (head.getData().(*addressInfo))

	//Add entry to the tree
	addressInfo.peerConn = peerConn
}

func (db *DB) getDestConn(token string) *connInfo {

	return db.destConns[token]
}

//loads a peer connection into the DB, replacing the existing one, if it exists
func (db *DB) addDestConnToDB(token string, destConn *connInfo) {

	db.destConns[token] = destConn
}

func (db *DB) removeDestConnFromDB(token string) {
	delete(db.destConns, token)
}

func (db *DB) getLastRoutingEntries(fromBlock uint64) []*routingEntry {
	return db.routingEntriesStack.peekFromBlock(fromBlock)
}

//Adds a new destination (shared by a peer) to the DB if it's better than the entry we have stored
func (db *DB) addNewDestinationToDB(destination *destination, neighbourPubKey [33]byte, lnClient *lndwrapper.Lnd) {

	//If we are trying to add information about ourselves, skip
	if destination.address == db.getLocalAddress() {
		return
	}

	//Get the routing address of the peer and the block Height so we can add new routing entries to the DB
	peerAddress, _ := db.GetNodeAddress(neighbourPubKey)
	blockHeight := db.getBlockHeight()

	//Build the new routing entry
	newEntry := &routingEntry{destination: destination.address, nextHop: peerAddress,
		capacity: destination.capacity, height: blockHeight}

	//Limit the new enrty capacities to the channel capacity
	localChannels := GetLocalChannels(lnClient)
	neighbourPubKeyString := PubKeyArrayToString(neighbourPubKey)
	for _, localChannel := range localChannels {
		//Found the channel shared with the next hop neighbour
		if localChannel.RemotePubkey == neighbourPubKeyString {
			if localChannel.LocalBalance < newEntry.capacity {
				newEntry.capacity = localChannel.LocalBalance
			}
		}
	}

	//Get the existing routing entry for this destination
	entry := db.getRoutingEntry(destination.address)

	//If there is no entry for this destination we just add a new one
	if entry == nil {
		db.addRoutingEntryToDB(newEntry)
		return
	}

	//If the destination shared with us by the neighbour has a worse capacity than the one we already have we do nothing
	if newEntry.capacity < entry.capacity {
		log.Println("Didn't update destination's next hop shared by", peerAddress)
		log.Println("Shared: Destination:", newEntry.destination, "Capacity:", newEntry.capacity)
		log.Println("Got: Destination:", entry.destination, "Capacity:", entry.capacity)
		return
	}

	//Add the new ypdated routing entry, it will replace the old one
	db.addRoutingEntryToDB(newEntry)
}

//SaveRoutingDBToFile writes the whole routing DB file.
//To be used on client exit
func (db *DB) SaveRoutingDBToFile() {

	routingEntries := db.routingEntriesStack.peekFromBlock(genesisBlock)
	var serializedRoutingEntry []byte
	var entry *routingEntry
	var routingDBPath = path.Join(db.filePath, routingDBFileName)

	//Open the routing database file
	log.Println("Writing Routing DB file...")
	fp, err := os.OpenFile(routingDBPath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer fp.Close()

	for _, entry = range routingEntries {

		//Serialize routing entry
		serializedRoutingEntry = serializeRoutingEntry(entry)

		//Write serialization to the file
		n, err := fp.Write(serializedRoutingEntry)
		log.Println("Wrote", n, "bytes to routing DB file.")
		if err != nil {
			log.Fatalln("Write failed:", err)
		}
	}
}

//UpdateAddressDB sincronizes the address database to the tip of the blockchain
func (db *DB) UpdateAddressDB(bitcoinCLient *bitcoindwrapper.Bitcoind, lnClient *lndwrapper.Lnd) {

	var lastScannedBlock = db.height
	var newAddressRegistrationList []*addressRegistration
	var newAddressInfo *addressInfo
	var validAddress bool
	var startingBlock = lastScannedBlock + 1

	log.Println("Starting DB update from block: " + strconv.FormatUint(lastScannedBlock, 10))

	//Get the number of blocks in the chain
	blockCount, err := GetBlockCount(bitcoinCLient)
	if err != nil {
		log.Fatal("Error getting block count:" + err.Error())
	}

	//If new blocks were found in the last 10 minutes we add the corresponding addresses to the DB
	if blockCount > lastScannedBlock {

		//Getting new address registrations starting from the block after the one we last lastScanned
		newAddressRegistrationList = getNewAddressRegistrations(bitcoinCLient, lnClient, startingBlock, blockCount)

		//Add every new valid address to the OpenAddressesDB
		for _, addressRegistration := range newAddressRegistrationList {
			validAddress, newAddressInfo = db.verifyAddressRegistration(addressRegistration, lnClient)
			if validAddress {
				db.addAddressToDB(newAddressInfo)
				db.saveAddressToFile(newAddressInfo)
			}
		}
	}

	//Update variables to reflect the update
	db.updateBlockHeight(blockCount)
	log.Println("Synced until block " + strconv.FormatUint(blockCount, 10))
}

//SynchronizeAddressDB is to be used as a new go routine to keep updating the address db in the background
func (db *DB) SynchronizeAddressDB(bitcoinCLient *bitcoindwrapper.Bitcoind, lnClient *lndwrapper.Lnd) {
	//Start update routine to keep the database updated
	var lastScannedBlock = db.height
	var newAddressRegistrationList []*addressRegistration
	var newAddressInfo *addressInfo
	var validAddress bool
	var startingBlock = lastScannedBlock + 1

	//Wait for new blocks and update the address DB accordingly every 10 minutes
	for {
		//Try to find new addresses everry 10 minutes (average block time)
		//Wait 10 minutes at the begging since this function is only called inside synchronizeDB
		//which syncs the database in a synchronous way
		time.Sleep(10 * time.Minute)

		log.Println("Starting DB update from block: " + strconv.FormatUint(lastScannedBlock, 10))

		//Get the number of blocks in the chain
		blockCount, err := GetBlockCount(bitcoinCLient)
		if err != nil {
			log.Fatal("Error getting block count:" + err.Error())
		}

		//If new blocks were found in the last 10 minutes we add the corresponding addresses to the DB
		if blockCount > lastScannedBlock {

			//Getting new address registrations starting from the block after the one we last lastScanned
			newAddressRegistrationList = getNewAddressRegistrations(bitcoinCLient, lnClient, startingBlock, blockCount)

			//Add every new valid address to the OpenAddressesDB
			for _, addressRegistration := range newAddressRegistrationList {

				validAddress, newAddressInfo = db.verifyAddressRegistration(addressRegistration, lnClient)
				if validAddress {
					db.addAddressToDB(newAddressInfo)
					db.saveAddressToFile(newAddressInfo)
				}
			}
		}

		//Update variables to reflect the update
		lastScannedBlock = blockCount
		startingBlock = lastScannedBlock + 1
		db.updateBlockHeight(lastScannedBlock)
		log.Println("Synced until block " + strconv.FormatUint(lastScannedBlock, 10))
	}
}

//SynchronizeRoutingDB updates the routing DB according to changes in the local channel balances
func (db *DB) SynchronizeRoutingDB(bitcoinCLient *bitcoindwrapper.Bitcoind, lnClient *lndwrapper.Lnd) {

	var registered bool
	var neighbourPubKey [33]byte
	var neighbourAddress [4]byte
	var localChannels []*lnrpc.Channel

	for {

		//Get the local channels
		localChannels = GetLocalChannels(lnClient)

		//Get all the routing entries
		routingEntries := db.routingEntriesStack.peekFromBlock(genesisBlock)

		//Iterate thourgh all the active channels of this node
		//Add routing entries for neighbours that are registered in the protocol
		for _, localChannel := range localChannels {

			neighbourPubKey = PubKeyStringToArray(localChannel.RemotePubkey)
			neighbourAddress, registered = db.GetNodeAddress(neighbourPubKey)

			//IF the channel is not registered we an skip it
			if !registered {
				continue
			}

			//Update routing entries whose next hop is the other end of this channel
			for n, routingEntry := range routingEntries {
				if routingEntry.nextHop == neighbourAddress && routingEntry.capacity > localChannel.LocalBalance {
					fmt.Println("Updated Entry #:", n)
					fmt.Println("Destination:", net.IP(routingEntry.destination[:]).String())
					fmt.Println("Next Hop:", net.IP(routingEntry.nextHop[:]).String())
					fmt.Println("Old Capacity:", routingEntry.capacity)
					fmt.Println("New Capacity:", localChannel.LocalBalance)
					routingEntry.capacity = localChannel.LocalBalance
				}
			}

			//Add destination to DB
			db.addNewDestinationToDB(&destination{address: neighbourAddress, capacity: localChannel.LocalBalance}, neighbourPubKey, lnClient)
		}

		//Update routing DB every minute
		time.Sleep(time.Minute)
	}
}

func (db *DB) verifyAddressRegistration(registration *addressRegistration, lnClient *lndwrapper.Lnd) (bool, *addressInfo) {

	sig := registration.sig[:]
	newAddress := registration.address

	//Verify the integrity of the signature and recover the pubkey of the node that signed it
	//Also verifies that the signature belongs to an active node in the database
	validSig, nodePubKey := VerifyMessage(lnClient, newAddress[:], sig)

	//If its an invalid signature we return
	if !validSig {
		log.Println("Verification failed for " + net.IP(newAddress[:]).String() + " registration: Invalid signature.")
		return false, nil
	}

	//Check if the address is already registered
	registeredFlag := db.IsAddressRegistered(newAddress)
	if registeredFlag {
		log.Println("Verification failed for " + net.IP(newAddress[:]).String() + " registration: Address already registered.")
		return false, nil
	}

	//Check if the node registering the address has any neighbors
	//and if it does, the address to be registered should be the first succeeding address, of an address that belongs to a one of the neighbours
	neighbors := GetNodeNeighboursPubKeys(lnClient, nodePubKey)
	if len(neighbors) > 0 {
		suggestedAddressFlag := false
		registeredNeighbourCounter := 0
		var neighbourAddress [4]byte
		var isRegistered bool
		var suggestedAddress [4]byte
		var err error

		for _, neighbor := range neighbors {

			// Check if node is registered in the protocol
			neighbourAddress, isRegistered = db.GetNodeAddress(neighbor)

			if isRegistered {
				//Get the first succeding address (suggested address) for a a node registering with this neighbour
				log.Println("Getting an address suggestion for " + net.IP(neighbourAddress[:]).String())
				suggestedAddress, err = db.SuggestAddress(neighbourAddress)
				if err != nil {
					log.Fatal(err)
				}
				log.Println("Suggestion was: " + net.IP(suggestedAddress[:]).String())
				log.Println("Want: " + net.IP(newAddress[:]).String())
				//To validate, check if the address sugggest to us is the registered address
				if suggestedAddress == newAddress {
					suggestedAddressFlag = true
				}
				registeredNeighbourCounter++
			}
		}

		//If there are registered neighbours and the address was not suggested by one of them we fail
		if registeredNeighbourCounter > 0 && !suggestedAddressFlag {
			log.Println("Verification failed for " + net.IP(newAddress[:]).String() + " registration: Address didn't follow a neighbour suggestion.")
			return false, nil
		}
	}

	validAddressInfo := addressInfo{address: registration.address, nodePubKey: nodePubKey,
		registrationHeight: registration.blockHeight, registrationTxID: registration.txID, version: registration.version}
	log.Println("Validated", net.IP(registration.address[:]).String())

	return true, &validAddressInfo
}

//PrintRoutingTable prints the routing table stored by this node
func (db *DB) PrintRoutingTable() {
	//Get all entries in the routing stack and print them
	routingEntries := db.routingEntriesStack.peekFromBlock(genesisBlock)

	for n, entry := range routingEntries {
		fmt.Println("Entry #:", n)
		fmt.Println("Destination:", net.IP(entry.destination[:]).String())
		fmt.Println("Next Hop:", net.IP(entry.nextHop[:]).String())
		fmt.Println("Capacity:", entry.capacity)
		fmt.Println("Update Height:", entry.height)
	}
}
