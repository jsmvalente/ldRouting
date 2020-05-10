package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/jsmvalente/lndRouting/bitcoindwrapper"
	. "github.com/jsmvalente/lndRouting/lnrlib"
	"github.com/lightningnetwork/lnd/lnrpc"
)

func main() {

	//var lastBlock uint32
	//var addressDB *DB
	var bitcoinClientHost string
	var bitcoinClientPortString string
	var bitcoinRPCUser string
	var bitcoinRPCPassword string
	var lightningClientHost string
	var lightningClientPortString string
	var macaroonPath string
	var tlsCertPath string
	var dataPath string
	var port string
	var localAddress [4]byte

	//Get values from command line arguments
	flag.StringVar(&bitcoinClientHost, "bitcoinClientHost", "localhost", "Bitcoin core host address")
	flag.StringVar(&bitcoinClientPortString, "bitcoinClientPort", "8332", "Bitcoin core RPC port")
	flag.StringVar(&bitcoinRPCUser, "bitcoinRPCUser", "rpcUserExample", "Bitcoin core RPC user")
	flag.StringVar(&bitcoinRPCPassword, "bitcoinRPCPassword", "rpcPasswordExample", "Bitcoin core RPC password")
	flag.StringVar(&lightningClientHost, "lightningClientHost", "localhost", "LND host address")
	flag.StringVar(&lightningClientPortString, "lightningClientPort", "10009", "LND host port")
	flag.StringVar(&port, "port", DefaultPort, "Port to listen for new connections to the client")
	flag.StringVar(&macaroonPath, "macaroonPath", path.Join(os.Getenv("HOME"), ".lnd/data/chain/bitcoin/mainnet/admin.macaroon"), "Path to the macaroon used with LND for authenticate")
	flag.StringVar(&tlsCertPath, "tlsCertPath", path.Join(os.Getenv("HOME"), ".lnd/tls.cert"), "Path to the TLS certificate used with LND for authentication")
	flag.StringVar(&dataPath, "dataPath", path.Join(os.Getenv("HOME"), ".lndRouting/data"), "Path to directory holding the application's data")
	flag.Parse()

	bitcoinClientPort, err := strconv.Atoi(bitcoinClientPortString)
	if err != nil {
		log.Fatal(err)
	}
	lightningClientPort, err := strconv.Atoi(lightningClientPortString)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connecting to bitcoin client")
	btcClient, err := ConnectToBitcoinClient(bitcoinClientHost, bitcoinClientPort, bitcoinRPCUser, bitcoinRPCPassword)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connecting to lightning network client")
	lnClient, err := ConnectToLNClient(lightningClientHost, lightningClientPort, macaroonPath, tlsCertPath)
	if err != nil {
		log.Fatal(err)
	}

	//Read our address database into memory
	log.Println("Reading addresses database")
	db := ReadDBFromDisk(dataPath, lnClient)

	// Update the database and start a subroutine to keep it keep up to database
	db.UpdateAddressDB(btcClient, lnClient)
	log.Println("Started sync address DB routine.")

	// synchronize the local routing entry DB with the changes that might happen
	//in the local balances
	go db.SynchronizeRoutingDB(btcClient, lnClient)
	log.Println("Started sync routing DB routine.")

	//Register a routing address if the user doesn't have one
	log.Println("Verifying local address registration...")
	localAddress, valid := verifyLocalAddressRegistration(btcClient, lnClient, db)
	if !valid {
		//Enter the address regitration menu to get the user to register an address
		localAddress = addressRegistrationMenu(btcClient, lnClient, db)
	}
	db.SaveLocalAddress(localAddress)

	//Tries to connect to LN peers that share a channel and are registered by using their
	//lightning node public IP addresses
	log.Println("Auto connecting to peers")
	ConnectToPeersAuto(lnClient, db)

	//Listen to new nodes that might want to connect with the client
	log.Println("Listening for incoming connections...")
	go ListenForConnections(lnClient, port, db)

	setupSigTermHandler(db)

	optionMenu(lnClient, db)
}

func verifyLocalAddressRegistration(btcClient *bitcoindwrapper.Bitcoind, lnClient lnrpc.LightningClient, addressDB *DB) ([4]byte, bool) {

	localNodePubKey := GetLocalNodePubKey(lnClient)
	localAddress, valid := addressDB.GetNodeAddress(localNodePubKey)

	// The local node already registered an address
	if !valid {
		log.Println("Didn't find an address associated with this node")
		return [4]byte{}, false
	}

	log.Println("Local address: " + net.IP(localAddress[:]).String())

	return localAddress, true
}

//Address registration process
func addressRegistrationMenu(btcClient *bitcoindwrapper.Bitcoind, lnClient lnrpc.LightningClient, addressDB *DB) [4]byte {

	type addressOption struct {
		suggested [4]byte
		neighbor  [4]byte
	}

	var addressOptions []addressOption

	neighborsPubKey := GetLocalNodeNeighboursPubKeys(lnClient)

	// Get one suggested address for each neighbor
	for _, neighborPubKey := range neighborsPubKey {

		neighborAddress, isRegistered := addressDB.GetNodeAddress(neighborPubKey)

		// Check if node is registered in the protocol
		if isRegistered {
			log.Println("Getting an address suggestion for " + net.IP(neighborAddress[:]).String())
			suggestedAddress, err := addressDB.SuggestAddress(neighborAddress)
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Suggestion was: " + net.IP(suggestedAddress[:]).String())

			//Save suggested addresses
			addressOptions = append(addressOptions, addressOption{suggested: suggestedAddress, neighbor: neighborAddress})
		}
	}

	//Print address options and let user decide the address he wants to register
	// If user doesn't have registered neighbors so he can just register any address he wants
	if len(addressOptions) > 0 {

		fmt.Println("\nSuggested addresses:\n ")
		fmt.Println("#\tSuggested address\tNeighbour address")
		for i, addressOption := range addressOptions {
			fmt.Println(strconv.Itoa(i) + "\t" + net.IP(addressOption.suggested[:]).String() + "\t\t\t" + net.IP(addressOption.neighbor[:]).String())
		}
		fmt.Println("\nChoose an address # or '-1' to register a non suggested one:")

		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		userRegistrationOptionString := strings.TrimSuffix(text, "\n")
		userRegistrationOption, err := strconv.Atoi(userRegistrationOptionString)
		if err != nil {
			log.Fatal(err)
		} else if userRegistrationOption < -1 || userRegistrationOption > len(addressOptions)-1 {
			log.Fatal("Invalid option")
		} else if userRegistrationOption == -1 {
			fmt.Println("You choose to register a non suggested address. This is not recommended.")
			return registerAddressMenu(btcClient, lnClient)
		} else {
			hash, err := BroadcastNewAddressTx(btcClient, lnClient, addressOptions[userRegistrationOption].suggested)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Registered '" + net.IP(addressOptions[userRegistrationOption].suggested[:]).String() + "'." + "\nTx Hash: " + hash)
			return addressOptions[userRegistrationOption].suggested
		}
	} else {
		fmt.Println("No registered neigbours, please register a new address")
		return registerAddressMenu(btcClient, lnClient)
	}

	return [4]byte{}
}

func getValidAddressFromUser() [4]byte {

	//Check if we should prompt the address to the user
	var addressString string
	var address, validIPAddress net.IP
	var validAddress [4]byte
	reader := bufio.NewReader(os.Stdin)

	//Keep prompting the user until we get a valid address
	for {
		fmt.Println("Please enter an address of the format '192.213.1.76', where the numbers between the dots are between 0 and 255")

		readText, _ := reader.ReadString('\n')
		addressString = strings.TrimSuffix(readText, "\n")

		//Validate address format
		address = net.ParseIP(addressString)
		validIPAddress = address.To4()

		if validIPAddress != nil {
			copy(validAddress[:], validIPAddress)
			return validAddress
		}

		fmt.Println("Invalid string format for '" + addressString + "'")
	}
}

//Registers a new address and if the address to be registered is set to nil prompts the user for it
func registerAddressMenu(btcClient *bitcoindwrapper.Bitcoind, lnClient lnrpc.LightningClient) [4]byte {

	//Check if we should prompt the address to the user
	address := getValidAddressFromUser()
	hash, err := BroadcastNewAddressTx(btcClient, lnClient, address)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Registered '", address, "'. Tx Hash: ", hash)
	return address
}

// Present an option menu to the user
func optionMenu(lnClient lnrpc.LightningClient, addressDB *DB) {

	//Present a menu to the User
	for true {
		fmt.Println("====================================================")
		fmt.Println("Welcome to the Lightning Distributed Routing Client")
		fmt.Println("====================================================")
		fmt.Print("\nThis software is still in its early stages of development. Use it at your own risk.\n\n")
		fmt.Println("1 - Find Route Auto")
		fmt.Println("2 - Find Route Manual")
		fmt.Println("3 - Connect to Peer")
		fmt.Println("4 - Send Payment")
		fmt.Println("5 - Print Routing Table")
		fmt.Println("6 - Find routing node lightning's public key")
		fmt.Println("0 - Exit")

		//Read from command line
		reader := bufio.NewReader(os.Stdin)
		readText, _ := reader.ReadString('\n')
		menuOptionString := strings.TrimSuffix(readText, "\n")
		menuOption, err := strconv.Atoi(menuOptionString)
		if err != nil {
			log.Fatal(err)
		}

		switch menuOption {
		case 1:

			fmt.Println("Receiver's LDR address:")
			address := getValidAddressFromUser()
			route, err := GetRouteAuto(lnClient, addressDB, address)
			if err != nil {
				log.Fatal(err)
			}
			PrintRoute(route)
		case 2:
			fmt.Println("Receiver's LDR address:")
			address := getValidAddressFromUser()
			fmt.Println("Receiver's IP address: (e.g. '192.1.3.56:8695)")
			fmt.Println("PS: 8695 is the default port.")
			readText, _ = reader.ReadString('\n')
			ipAddress := strings.TrimSuffix(readText, "\n")
			route, err := GetRouteManual(lnClient, addressDB, address, ipAddress)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Got Route!")
			PrintRoute(route)
		case 3:
			//Get an address from the user
			fmt.Println("Enter the IP 'address:port' of the peer you're trying to connect to, e.g. '192.1.3.56:8695")
			fmt.Println("PS: 8695 is the default port.")
			readText, _ = reader.ReadString('\n')
			address := strings.TrimSuffix(readText, "\n")
			ConnectToPeer(lnClient, addressDB, address)
		case 4:
		case 5:
			fmt.Println("Printing routing table")
			addressDB.PrintRoutingTable()
		case 6:
			fmt.Println("LDR address for which you want the associated node's public key:")
			address := getValidAddressFromUser()
			if !addressDB.IsAddressRegistered(address) {
				fmt.Println(address, "is not a registered address")
			} else {
				pubKeyArray := addressDB.GetAddressNode(address)
				fmt.Println(PubKeyArrayToString(pubKeyArray))
			}
		case 0:
			addressDB.SaveRoutingDBToFile()
			os.Exit(0)
		}
	}
}

func unlockWalletMenu(btcClient *bitcoindwrapper.Bitcoind) {
	for {
		fmt.Println("Bitcoin wallet passphrase: (unlocking bitcoin client is necessary in order to register a new lightning address)")

		//Read from command line
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		passphrase := strings.TrimSuffix(text, "\n")
		err := UnlockWallet(btcClient, passphrase)
		if err == nil {
			break
		} else {
			log.Println(err)
		}
	}
}

func setupSigTermHandler(db *DB) {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r- Ctrl+C pressed in Terminal")
		db.SaveRoutingDBToFile()
		os.Exit(0)
	}()
}
