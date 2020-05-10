package lndrlib

import (
	"encoding/binary"
	"encoding/hex"
	"log"
)

func routingEntryToDestination(entry *routingEntry) *destination {
	dest := &destination{}

	dest.address = entry.destination
	dest.capacity = entry.capacity

	return dest
}

func destinationToRoutingEntry(dest *destination, currentBlock uint64, nextHopNeighbour [4]byte) *routingEntry {
	entry := &routingEntry{}

	entry.destination = dest.address
	entry.capacity = dest.capacity
	entry.height = currentBlock
	entry.nextHop = nextHopNeighbour

	return entry
}

//PubKeyArrayToString transforms a 33 byte public key to its hex encoded string form
func PubKeyArrayToString(pubkey [33]byte) string {
	return hex.EncodeToString(pubkey[:])
}

//PubKeyStringToArray transforms a hex encoded string public key to its 33 byte form
func PubKeyStringToArray(pubkey string) [33]byte {

	pubKeyArray := [33]byte{}
	pubKeySlice, err := hex.DecodeString(pubkey)
	if err != nil {
		log.Fatal(err)
	}

	copy(pubKeyArray[:], pubKeySlice)

	return pubKeyArray
}

func bitToByte(addressBits [32]bool) [4]byte {

	addressBytes := [4]byte{}
	var multiplier byte
	var addressByte byte

	for a := 0; a < len(addressBytes); a++ {

		//Reset multiplier and byte
		multiplier = 0x80
		addressByte = 0x00

		for b := 0; b < len(addressBits)/len(addressBytes); b++ {

			if addressBits[a*(len(addressBits)/len(addressBytes))+b] {
				addressByte = addressByte | multiplier
			}
			//Bitwise shift the multiplier
			multiplier = multiplier >> 1

		}

		//Save the byte
		addressBytes[a] = addressByte
	}

	return addressBytes
}

//byteToBit transforms a 4 byte address array into its 32 bit bool representation
func byteToBit(address [4]byte) [32]bool {

	addressBits := [32]bool{}
	var mask byte
	var addressByte byte

	for a := 0; a < len(address); a++ {

		mask = byte(0x80)
		addressByte = address[a]

		for b := 0; b < len(addressBits)/len(address); b++ {
			if mask&addressByte == mask {
				addressBits[(8*a)+b] = true
			} else {
				addressBits[(8*a)+b] = false
			}
			mask = mask >> 1
		}
	}

	return addressBits
}

func serializeRoutingEntry(entry *routingEntry) []byte {
	var buf []byte

	// Serialize in the following order: destination, hop, capacity, height
	buf = append(buf, entry.destination[:]...)
	buf = append(buf, entry.nextHop[:]...)

	capacityBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(capacityBytes, uint64(entry.capacity))
	buf = append(buf, capacityBytes...)

	blockHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(blockHeightBytes, entry.height)
	buf = append(buf, blockHeightBytes...)

	return buf
}

func deserializeRoutingEntry(entryBytes []byte) *routingEntry {
	entry := routingEntry{}

	copy(entry.destination[:], entryBytes[0:4])
	copy(entry.nextHop[:], entryBytes[4:8])
	entry.capacity = int64(binary.LittleEndian.Uint64(entryBytes[8:16]))
	entry.height = binary.LittleEndian.Uint64(entryBytes[16:24])

	return &entry
}

func serializeDestination(dest *destination) []byte {
	var buf []byte

	// Serialize in the following order: destination, capacity
	buf = append(buf, dest.address[:]...)

	capacityBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(capacityBytes, uint64(dest.capacity))
	buf = append(buf, capacityBytes...)

	return buf
}

func deserializeDestination(destBytes []byte) *destination {
	dest := destination{}

	copy(dest.address[:], destBytes[0:4])
	dest.capacity = int64(binary.LittleEndian.Uint64(destBytes[4:12]))

	return &dest
}

func serializeBlockHeight(blockHeight uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, blockHeight)

	return buf
}

func deserializeBlockHeight(blockHeightBytes []byte) uint64 {
	blockHeight := binary.LittleEndian.Uint64(blockHeightBytes)
	return blockHeight
}

func serializeAddressInfo(info *addressInfo) []byte {
	var buf []byte

	// Serialize in the following order: address, nodePubKey, blockHeight, txID, version)
	buf = append(buf, info.address[:]...)
	buf = append(buf, info.nodePubKey[:]...)

	blockHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(blockHeightBytes, info.registrationHeight)
	buf = append(buf, blockHeightBytes...)

	buf = append(buf, info.registrationTxID[:]...)

	versionBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(versionBytes, info.version)
	buf = append(buf, versionBytes...)

	if len(buf) != addressInfoSerializedSize {
		log.Fatalln("Error serializing addressInfo: inconsistent sizes.")
	}

	return buf
}

// Deserialize in the following order: address, nodePubKey, blockHeight, txID, version)
func deserializeAddressInfo(infoBytes []byte) *addressInfo {

	info := addressInfo{}

	copy(info.address[:], infoBytes[0:4])
	copy(info.nodePubKey[:], infoBytes[4:37])
	info.registrationHeight = binary.LittleEndian.Uint64(infoBytes[37:45])
	copy(info.registrationTxID[:], infoBytes[36:68])
	info.version = binary.LittleEndian.Uint32(infoBytes[45:49])

	return &info
}

func serializeRoute(route *Route) []byte {

	var serializedRoute []byte

	//Start building the header, first with the number of hops (2 bytes)
	numberHopsBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(numberHopsBytes, uint16(len(route.hops)))
	serializedRoute = append(serializedRoute, numberHopsBytes...)

	//Add the destination to the header (4 bytes)
	serializedRoute = append(serializedRoute, route.destination[:]...)

	//Add route token to the header (10 bytes)
	serializedRoute = append(serializedRoute, []byte(route.token)...)

	//Add route capacity to the header (8 bytes)
	capacityBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(capacityBytes, uint64(route.capacity))
	serializedRoute = append(serializedRoute, capacityBytes...)

	for i := 0; i < len(route.hops); i++ {
		serializedRoute = append(serializedRoute, route.hops[i][:]...)
	}

	return serializedRoute
}

func deserializeRoute(routeBytes []byte) *Route {

	route := &Route{}
	var hop [4]byte

	numberHops := binary.LittleEndian.Uint16(routeBytes[0:2])
	copy(route.destination[:], routeBytes[2:6])
	route.token = string(routeBytes[6:16])
	route.capacity = int64(binary.LittleEndian.Uint64(routeBytes[16:24]))

	log.Println("# hops:", numberHops)

	for i := 0; i < int(numberHops); i++ {
		copy(hop[:], routeBytes[24+4*i:24+4*(i+1)])
		route.hops = append(route.hops, hop)
	}

	return route
}
