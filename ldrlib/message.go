package ldrlib

import (
	"encoding/binary"
	"errors"
	"log"

	"github.com/jsmvalente/ldRouting/lndwrapper"
)

const (
	tableRequestType  uint16 = 0
	tableResponseType uint16 = 1
	forwardRouteType  uint16 = 2

	//The size of a table request header (in bytes)
	tableRequestHeaderSize = 8
	//The size of the type of message (in bytes)
	messageTypeSize = 2
	//The size for a table response header (in bytes)
	tableResponseHeaderSize = 2
	//The size for a forward route header (in bytes)
	forwardRouteHeaderSize = 24
	//Size for a routingMessage  (in bytes)
	destinationSize = 12
)

//Destination holds a routing destination and its corresponding capacity
//to be shared with a peer
//destination: the destination node's address
//capacity: the known minimum capacity for this route
type destination struct {
	address  [4]byte
	capacity int64
}

func createForwardRouteMessage(route *Route) ([]byte, error) {

	var message []byte

	//Add message type
	messageTypeBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(messageTypeBytes, forwardRouteType)
	message = append(message, messageTypeBytes...)

	message = append(message, serializeRoute(route)...)

	return message, nil
}

func processForwardRouteMessage(message []byte) (*Route, error) {

	//Check if the response has enough length for it to be valid
	if len(message) < messageTypeSize+forwardRouteHeaderSize {
		return nil, errors.New("Invalid forward message size")
	}

	//Extract the type of message
	messageTypeBytes := message[:2]
	responseType := binary.BigEndian.Uint16(messageTypeBytes)

	//Validate the type of message
	if responseType != forwardRouteType {
		return nil, errors.New("Invalid forward route message type")
	}

	return deserializeRoute(message[2:]), nil
}

//Create a serialized table request
func createTableRequest(startingBlock uint64) ([]byte, error) {

	var request []byte

	if startingBlock < genesisBlock {
		return nil, errors.New("Starting block is too old")
	}

	//Serialize the message type and append it to the response
	messageTypeBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(messageTypeBytes, tableRequestType)

	request = append(request, messageTypeBytes...)

	//Serialize the startingBloc  and append it to the response
	startingBlockBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(startingBlockBytes, startingBlock)

	request = append(request, startingBlockBytes...)

	return request, nil
}

// Generates the response for a certain table request
func processTableRequest(db *DB, request []byte) ([]byte, error) {

	var response []byte
	var serializedDestination []byte
	var destination *destination

	//Validate the length of the request
	if len(request) != messageTypeSize+tableRequestHeaderSize {
		return nil, errors.New("Invalid table request size")
	}

	//Extract the type of message
	requestTypeBytes := request[:2]
	requestType := binary.BigEndian.Uint16(requestTypeBytes)

	//Validate the type of message
	if requestType != tableRequestType {
		return nil, errors.New("Invalid table request message type")
	}

	//Read the starting block the peer asked for (uint64 - 8 bytes)
	startingBlockBytes := request[2:10]
	startingBlock := binary.BigEndian.Uint64(startingBlockBytes)

	if startingBlock < genesisBlock {
		return nil, errors.New("Starting block is too old")
	}

	//Serialize message type and add it to the response
	responseTypeBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(responseTypeBytes, tableResponseType)
	response = append(response, responseTypeBytes...)

	//Get the routing entries to be sent and
	//transform them into hops so they can be shared with the peer
	routingEntries := db.getLastRoutingEntries(startingBlock)
	entriesCount := len(routingEntries)

	//Serialize routing message count and add it to the response
	entriesCountBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(entriesCountBytes, uint16(entriesCount))
	response = append(response, entriesCountBytes...)

	for _, entry := range routingEntries {
		destination = routingEntryToDestination(entry)
		serializedDestination = serializeDestination(destination)
		response = append(response, serializedDestination...)
		log.Println("Table response adding destination:", destination)
	}

	return response, nil
}

//Processes the response of a previously made table request
func processTableResponse(response []byte, db *DB, peerPubKey [33]byte, lnClient *lndwrapper.Lnd) error {

	var dest *destination
	var serializedDest []byte

	//Check if the response has enough length for it to be valid
	if len(response) < messageTypeSize+tableResponseHeaderSize {
		return errors.New("Invalid table response message size")
	}

	//Extract the type of message
	responseTypeBytes := response[:2]
	responseType := binary.BigEndian.Uint16(responseTypeBytes)

	//Validate the type of message
	if responseType != tableResponseType {
		return errors.New("Invalid table response message type")
	}

	//Extract the number of entries
	entriesCountBytes := response[2:4]
	entriesCount := int(binary.BigEndian.Uint16(entriesCountBytes))

	//Extract the entries and add them to the database
	for n := 0; n < entriesCount; n++ {
		serializedDest = response[4+n*destinationSize : 4+(n+1)*destinationSize]
		dest = deserializeDestination(serializedDest)
		log.Println("Adding destination to DB:", dest)
		db.addNewDestinationToDB(dest, peerPubKey, lnClient)
	}

	return nil
}
