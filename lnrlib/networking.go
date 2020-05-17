package lndrlib

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"sync"
	"time"

	"github.com/jsmvalente/ldRouting/lndwrapper"
)

const (
	peerConn        int8 = 0
	destinationConn int8 = 1

	//DefaultPort is the default tcp port
	DefaultPort string = "8695"
)

type connInfo struct {
	mutex      sync.Mutex
	conn       net.Conn
	sessionKey []byte
	baseIV     []byte
	seqNumber  []byte
}

//ForwardRoute forwards the route to the node identificated by the LDR address
func ForwardRoute(client *lndwrapper.Lnd, db *DB, route *Route, address [4]byte) {
	log.Println("Forwarding route:")
	PrintRoute(route)
	connInfo := db.getPeerConn(address)
	serializedRoute, _ := createForwardRouteMessage(route)
	nonce := getNonce(connInfo.baseIV, connInfo.seqNumber)
	encryptedRoute := encryptAES(connInfo.sessionKey, nonce, serializedRoute)
	encryptedRouteLength := uint16(len(encryptedRoute))
	encryptedRouteLengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(encryptedRouteLengthBytes, encryptedRouteLength)
	encryptedMessage := append(encryptedRouteLengthBytes, encryptedRoute...)
	_, err := connInfo.conn.Write(encryptedMessage)
	if err != nil {
		log.Println("Error writing:", err)
	}

	//Increment seqNumber to be used on the next message
	incrementSeqNumber(connInfo)
}

func sendRouteToSender(db *DB, route *Route) {
	connInfo := db.getDestConn(route.token)

	serializedRoute := serializeRoute(route)

	_, err := connInfo.conn.Write(serializedRoute)
	if err != nil {
		log.Println("Error writing:", err)
	}

	closeDestConnection(db, route.token)
}

func closeDestConnection(db *DB, token string) {
	db.getDestConn(token).conn.Close()
	db.removeDestConnFromDB(token)
}

//ReceiveRouteFromDestination waits on a connection
func ReceiveRouteFromDestination(db *DB, token string) *Route {

	//Read the number of hops
	numberHopsBytes := make([]byte, 2)
	_, err := db.getDestConn(token).conn.Read(numberHopsBytes)
	if err != nil {
		log.Println(err)
	}
	numberHops := binary.LittleEndian.Uint16(numberHopsBytes)

	//Read the rest of the route
	restRouteBytes := make([]byte, 4+10+8+numberHops*4)
	_, err = db.getDestConn(token).conn.Read(restRouteBytes)
	if err != nil {
		log.Println(err)
	}

	routeBytes := append(numberHopsBytes, restRouteBytes...)

	closeDestConnection(db, token)

	return deserializeRoute(routeBytes)
}

//ConnectToPeersAuto - Connects to peers connects to peers automatically by trying to use
//their lightning nodes ip addresses
func ConnectToPeersAuto(client *lndwrapper.Lnd, db *DB) {

	var neighborIPs []string
	var err error

	neighbors := GetLocalNodeNeighboursPubKeys(client)

	// Get IP for each neighbor
	for _, neighbor := range neighbors {

		// Check if node is registered in the protocol
		if db.IsNodeRegistered(neighbor) {
			//If it is we get the IP's for this node and try and connect to it
			neighborIPs = GetNodeIPs(client, neighbor)
			for _, ipAddress := range neighborIPs {
				log.Println("Trying to connect to", PubKeyArrayToString(neighbor), "@", neighborIPs[0]+DefaultPort)
				err = ConnectToPeer(client, db, ipAddress+":"+DefaultPort)
				if err != nil {
					log.Println(err)
				} else {
					break
				}
			}
		}
	}
}

//ConnectToPeer connects to a peer
func ConnectToPeer(client *lndwrapper.Lnd, db *DB, ipAddress string) error {
	conn, err := net.Dial("tcp", ipAddress)
	if err != nil {
		return err
	}
	writeConnectionType(conn, peerConn)
	log.Println("Connected to:" + conn.RemoteAddr().String())
	sessionKey, baseIV, startSeq, peerLightningKey := offerPeerHandshake(conn, client, db)
	log.Println("Peer Handshake successful")

	go handlePeerConnection(conn, client, db, sessionKey, baseIV, startSeq, peerLightningKey)

	return nil
}

//ConnectToDestinationAuto connects to a destination node using its IP
func ConnectToDestinationAuto(client *lndwrapper.Lnd, db *DB, address [4]byte, routeToken string) {

	var err error

	if !db.IsAddressRegistered(address) {
		log.Println("Trying to connect to unregistered address", address)
		return
	}
	destinationPubKey := db.GetAddressNode(address)
	neighborIPs := GetNodeIPs(client, destinationPubKey)

	for _, ipAddress := range neighborIPs {
		log.Println("Trying to connect to", PubKeyArrayToString(destinationPubKey), "@", ipAddress+":"+DefaultPort)
		err = ConnectToDestination(client, db, address, ipAddress+":"+DefaultPort, routeToken)
		if err != nil {
			log.Println(err)
		} else {
			break
		}
	}
}

//ConnectToDestination connects to a destination node using the provided IP
func ConnectToDestination(client *lndwrapper.Lnd, db *DB, address [4]byte, ipAddress string, routeToken string) error {
	conn, err := net.Dial("tcp", ipAddress)
	if err != nil {
		return err
	}
	writeConnectionType(conn, destinationConn)
	log.Println("Connected to:" + conn.RemoteAddr().String())

	//Send code for route request
	routeTokenBytes := []byte(routeToken)
	_, err = conn.Write(routeTokenBytes)
	if err != nil {
		return err
	}
	db.addDestConnToDB(routeToken, &connInfo{conn: conn})

	return nil
}

//ListenForConnections listens to new nodes that want to connect and accepts them
func ListenForConnections(lnClient *lndwrapper.Lnd, port string, db *DB) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Println(err)
	}

	log.Println("Listening on port", port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
		} else {
			log.Println("Accepted new connection from:" + conn.RemoteAddr().String())

			connType := readConnectionType(conn)

			if connType == peerConn {

				log.Println("Peer Connection, accepting handshake...")
				sessionKey, baseIV, startSeq, lightningPeerPubKey := acceptPeerHandshake(conn, lnClient, db)
				log.Println("Peer Handshake successful")

				//Handle the connection
				go handlePeerConnection(conn, lnClient, db, sessionKey, baseIV, startSeq, lightningPeerPubKey)

			} else if connType == destinationConn {
				//Read connecting token and save connection in the Database
				routeTokenBytes := make([]byte, 10)
				_, err = conn.Read(routeTokenBytes)
				if err != nil {
					log.Println(err)
				}
				routeToken := string(routeTokenBytes)
				db.addDestConnToDB(routeToken, &connInfo{conn: conn})
			} else {
				log.Println("Unknown connection type")
			}

		}
	}
}

func readConnectionType(conn net.Conn) int8 {

	var connType int8
	err := binary.Read(conn, binary.LittleEndian, &connType)
	if err != nil {
		log.Println(err)
	}

	return connType
}

func writeConnectionType(conn net.Conn, connType int8) {
	err := binary.Write(conn, binary.LittleEndian, connType)
	if err != nil {
		log.Println(err)
	}
}

func offerDestinationHandshake(conn net.Conn, client *lndwrapper.Lnd, addressDB *DB) ([]byte, []byte) {

	//Create new RSA public key that will be used to encrypt the eoute data
	privKey, pubKey := generateRSAKeyPair()

	return privKey, pubKey
}

func acceptDestinationHandshake(conn net.Conn, client *lndwrapper.Lnd, addressDB *DB) {

}

func offerPeerHandshake(conn net.Conn, client *lndwrapper.Lnd, addressDB *DB) ([]byte, []byte, []byte, [33]byte) {

	//Create new RSA public key that will be used to encrypt the
	//simmetrical AES key
	privKey, pubKey := generateRSAKeyPair()

	//The public key is signed by the lighting node
	pubKeySignature := SignMessage(client, pubKey)
	log.Println("pubKeySignature size: ", len(pubKeySignature))

	//The public key and signature are sent to the peer
	log.Println("Sending pubkey + pubKeySignature")
	pubKeyAndSignature := append(pubKey, pubKeySignature...)
	_, err := conn.Write(pubKeyAndSignature)
	if err != nil {
		log.Println("Error writing:", err)
	}

	//Read the 65 byte signature sent by the peer
	log.Println("Reading public key + signature")
	peerPubKeyAndSignature := make([]byte, RSAKeySize+SignatureSize)
	_, err = conn.Read(peerPubKeyAndSignature)
	if err != nil {
		log.Println(err)
	}

	peerPubKey := peerPubKeyAndSignature[:RSAKeySize]
	peerPubKeySignature := peerPubKeyAndSignature[RSAKeySize:]

	//Verify the signature
	//Since this VerifyMessage implementation uses message verification by the lighting node
	//it also ensures the active node in the resident node's channel database
	verified, peerLightningPubKey := VerifyMessage(client, peerPubKey, peerPubKeySignature)

	if !verified {
		log.Fatalln("Failed when verifying peer's signature.")
	}

	log.Println("Verified signature with", PubKeyArrayToString(peerLightningPubKey))

	//Verify that the peer node shares a channel with the local node
	neighbors := GetLocalNodeNeighboursPubKeys(client)
	sharesChannelFlag := false
	for _, neighbor := range neighbors {
		if neighbor == peerLightningPubKey {
			sharesChannelFlag = true
			break
		}
	}
	if !sharesChannelFlag {
		log.Fatalln("Peer does not share a channel with the local node.")
	}

	//Verify that the peer node is also registered in the routing protocol
	if !addressDB.IsNodeRegistered(peerLightningPubKey) {
		log.Fatalln("Peer is not registered in the routing protocol.")
	}

	//Create an AES session key, nonce base IV and start seq number.
	//appending them and encrypting it using the peer's RSA pub key
	log.Println("Creating AES session info.")
	aesKey, err := createAESKey()
	if err != nil {
		log.Println(err)
	}
	baseIV, err := generateNRandomBytes(AESBaseIVSize)
	if err != nil {
		log.Println(err)
	}
	startSeq, err := generateNRandomBytes(AESStartSeqSize)
	if err != nil {
		log.Println(err)
	}
	nonceInfo := append(baseIV, startSeq...)
	aesInfo := append(aesKey, nonceInfo...)
	log.Println("Key:", aesKey)
	log.Println("baseIV:", baseIV)
	log.Println("startSeq:", startSeq)
	log.Println("Encrypting AES info with RSA key.")

	encryptedAESInfo := encryptRSA(peerPubKey, aesInfo)

	//Send the encryted AES session key to the peer
	log.Println("Sharing session key, base IV and starting sequence number with " + PubKeyArrayToString(peerLightningPubKey))
	_, err = conn.Write(encryptedAESInfo)
	if err != nil {
		log.Println("Error writing:", err)
	}

	//The peer sends back the AES Key as an ACK
	log.Println("Reading AES encrypted Key from peer")
	encryptedAESKeyMessage := make([]byte, RSAEncryptionSize)
	_, err = conn.Read(encryptedAESKeyMessage)
	if err != nil {
		log.Println(err)
	}

	log.Println("Decrypting AES key with RSA privkey.")
	aesKeyMessage := decryptRSA(privKey, encryptedAESKeyMessage)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Decrypted:", aesKeyMessage)
	log.Println("Should be:", aesKey)
	if bytes.Compare(aesKey, aesKeyMessage) != 0 {
		log.Fatal("Error reading AES ACK from peer")
	}

	return aesKey, baseIV, startSeq, peerLightningPubKey

}

func acceptPeerHandshake(conn net.Conn, client *lndwrapper.Lnd, db *DB) ([]byte, []byte, []byte, [33]byte) {

	//Read the 65 byte signature sent by the peer
	log.Println("Reading public key signature")
	peerPubKeyAndSignature := make([]byte, RSAKeySize+SignatureSize)
	_, err := conn.Read(peerPubKeyAndSignature)
	if err != nil {
		log.Println(err)
	}

	peerPubKey := peerPubKeyAndSignature[:RSAKeySize]
	peerPubKeySignature := peerPubKeyAndSignature[RSAKeySize:]

	//Verify the signature
	//Since this VerifyMessage implementation uses message verification by the lighting node
	//it also ensures the active node in the resident node's channel database
	verified, peerLightningPubKey := VerifyMessage(client, peerPubKey, peerPubKeySignature)

	if !verified {
		log.Fatalln("Failed when verifying peer's signature.")
	}

	log.Println("Verified signature with", PubKeyArrayToString(peerLightningPubKey))

	//Verify that the peer node shares a channel with the local node
	neighbors := GetLocalNodeNeighboursPubKeys(client)
	sharesChannelFlag := false
	for _, neighbor := range neighbors {
		if neighbor == peerLightningPubKey {
			sharesChannelFlag = true
			break
		}
	}
	if !sharesChannelFlag {
		log.Fatalln("Peer does not share a channel with the local node.")
	}
	//Verify that the peer node is also registered in the routing protocol
	if !db.IsNodeRegistered(peerLightningPubKey) {
		log.Fatalln("Peer's is not registered in the routing protocol.")
	}

	//Create new RSA public key that will be used to encrypt the
	//simmetrical AES key ACK
	privKey, pubKey := generateRSAKeyPair()

	//The public key is signed by the lighting node
	pubKeySignature := SignMessage(client, pubKey)

	//The public key is sent to the peer
	log.Println("Sending pubkey + pubKeySignature")
	pubKeyAndSignature := append(pubKey, pubKeySignature...)
	_, err = conn.Write(pubKeyAndSignature)
	if err != nil {
		log.Println("Error writing:", err)
	}

	//Read AES session key sent by the peer
	encryptedAESInfoMessage := make([]byte, 256)
	log.Println("Reading AES Info from peer")
	_, err = conn.Read(encryptedAESInfoMessage)
	if err != nil {
		log.Println(err)
	}

	log.Println("Decrypting AES info with RSA privkey")
	aesInfo := decryptRSA(privKey, encryptedAESInfoMessage)
	aesKey := aesInfo[:AESKeySize]
	baseIV := aesInfo[AESKeySize : AESKeySize+AESBaseIVSize]
	startSeq := aesInfo[AESKeySize+AESBaseIVSize : AESKeySize+AESBaseIVSize+AESStartSeqSize]
	log.Println("Key:", aesKey)
	log.Println("baseIV:", baseIV)
	log.Println("startSeq:", startSeq)

	//Send the encryted AES session key to the peer
	log.Println("Encrypting AES key with RSA pubkey")
	encryptedAESKey := encryptRSA(peerPubKey, aesKey)
	log.Println("Sending AES session key (ACK):", aesKey)
	_, err = conn.Write(encryptedAESKey)
	if err != nil {
		log.Println("Error writing:", err)
	}

	return aesKey, baseIV, startSeq, peerLightningPubKey
}

func handlePeerConnection(conn net.Conn, lnClient *lndwrapper.Lnd, db *DB, sessionKey []byte, baseIV []byte, startSeq []byte, peerPubKey [33]byte) {

	var err error
	var encryptedMessageLength uint16
	var encryptedMessageLengthBytes []byte
	var encryptedMessage []byte
	var message []byte
	var messageTypeBytes []byte
	var messageType uint16
	var nonce []byte
	var response []byte
	var encryptedResponse []byte
	var encryptedResponseLength uint16
	var encryptedResponseLengthBytes []byte
	var encryptedMessageResponse []byte
	var route *Route

	//Save the connection in memory
	peerConnInfo := &connInfo{conn: conn, sessionKey: sessionKey, baseIV: baseIV, seqNumber: startSeq}
	address, _ := db.GetNodeAddress(peerPubKey)
	db.addPeerConnToDB(address, peerConnInfo)

	//Start sending periodic table requests for this peer
	log.Println("Setting up periodic table requests")
	go sendTableRequestPeriodically(db, address)

	//Treat received messages for this connectin in a loop
	for {
		//Read the length of the encrypted message in the buffer
		encryptedMessageLengthBytes = make([]byte, 2)
		_, err = conn.Read(encryptedMessageLengthBytes)
		if err != nil {
			log.Println(err)
			return
		}
		encryptedMessageLength = binary.BigEndian.Uint16(encryptedMessageLengthBytes)

		//Read the encryped data
		encryptedMessage = make([]byte, encryptedMessageLength)
		_, err = conn.Read(encryptedMessage)
		if err != nil {
			log.Println(err)
			return
		}
		//Lock the thread so we use the right seq number
		peerConnInfo.mutex.Lock()

		log.Println("Got a new message, decrypting it with seq number", peerConnInfo.seqNumber)
		//Get the decrypted message bytes
		nonce = getNonce(baseIV, peerConnInfo.seqNumber)
		message = decryptAES(sessionKey, nonce, encryptedMessage)
		log.Println("Message decrypted")

		//Increment seqNumber to be used on the next message
		incrementSeqNumber(peerConnInfo)

		//Extract the type of message
		messageTypeBytes = message[:2]
		messageType = binary.BigEndian.Uint16(messageTypeBytes)

		//Act accordingt to the type of message
		//Requests will generate responses and responses will be processed
		if messageType == tableRequestType {
			log.Println("New Table Request")
			response, err = processTableRequest(db, message)
			if err != nil {
				log.Println(err)
				return
			}

		} else if messageType == tableResponseType {
			log.Println("New Table Response")
			err = processTableResponse(message, db, peerPubKey)
			if err != nil {
				log.Println(err)
				return
			}

		} else if messageType == forwardRouteType {
			log.Println("New route forward request")
			route, err = processForwardRouteMessage(message)
			if err != nil {
				log.Println(err)
			}

			if route.destination == db.getLocalAddress() {
				sendRouteToSender(db, route)
			} else {
				//Add the first hop to the route and send forward the request through the network
				localHop, err := addHopToRoute(lnClient, db, route)
				if err != nil {
					log.Println(err)
				}
				ForwardRoute(lnClient, db, route, localHop)
			}

		} else {
			log.Println("Invalid message type")
			return
		}

		//If there is a response to the message the peer sent we send it
		if response != nil {
			//Encrypt and send the response preceded by its length
			nonce = getNonce(baseIV, peerConnInfo.seqNumber)
			encryptedResponse = encryptAES(sessionKey, nonce, response)
			encryptedResponseLength = uint16(len(encryptedResponse))
			encryptedResponseLengthBytes = make([]byte, 2)
			binary.BigEndian.PutUint16(encryptedResponseLengthBytes, encryptedResponseLength)
			encryptedMessageResponse = append(encryptedResponseLengthBytes, encryptedResponse...)
			_, err = conn.Write(encryptedMessageResponse)
			if err != nil {
				log.Println("Error writing:", err)
			}
			log.Println("Sent response using seq number", peerConnInfo.seqNumber)

			//Increment seqNumber to be used on the next message
			incrementSeqNumber(peerConnInfo)
			//Reset the response variable
			response = nil
		}

		//Unlock connection so it can be used again
		peerConnInfo.mutex.Unlock()
	}
}

func sendTableRequestPeriodically(db *DB, address [4]byte) {

	var request []byte
	var err error
	var nonce []byte
	var encryptedRequest []byte
	var encryptedRequestLength uint16
	var encryptedRequestLengthBytes []byte
	var encryptedMessage []byte
	connInfo := db.getPeerConn(address)

	for {
		request, err = createTableRequest(genesisBlock)
		if err != nil {
			log.Println(err)
			return
		}

		//Lock the thread using the corresponding mutex
		connInfo.mutex.Lock()
		log.Println("Sending new table request...")
		//Encrypt and send the response preceded by its length
		nonce = getNonce(connInfo.baseIV, connInfo.seqNumber)
		encryptedRequest = encryptAES(connInfo.sessionKey, nonce, request)
		encryptedRequestLength = uint16(len(encryptedRequest))
		encryptedRequestLengthBytes = make([]byte, 2)
		binary.BigEndian.PutUint16(encryptedRequestLengthBytes, encryptedRequestLength)
		encryptedMessage = append(encryptedRequestLengthBytes, encryptedRequest...)
		_, err = connInfo.conn.Write(encryptedMessage)
		if err != nil {
			log.Println("Error writing:", err)
		}
		log.Println("Sent create table request", address, "using seq number", connInfo.seqNumber)
		//Increment seqNumber to be used on the next message
		incrementSeqNumber(connInfo)

		//Unlock the thread using the corresponding mutex
		connInfo.mutex.Unlock()

		//WAit 10 minutes before sending the next table request
		time.Sleep(10 * time.Minute)
	}
}
