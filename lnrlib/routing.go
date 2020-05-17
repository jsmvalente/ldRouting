package lndrlib

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"

	"github.com/jsmvalente/ldRouting/lndwrapper"
)

//Route represents a payment route
type Route struct {
	destination [4]byte
	capacity    int64
	hops        [][4]byte
	token       string
}

func createRoute(destination [4]byte) *Route {
	routeTokenBytes := make([]byte, 10)
	rand.Read(routeTokenBytes)
	route := &Route{destination: destination, hops: [][4]byte{}, capacity: 0}
	route.token = string(routeTokenBytes)
	return route
}

//GetRouteAuto gets a route to a destination
func GetRouteAuto(client *lndwrapper.Lnd, db *DB, destination [4]byte) (*Route, error) {

	route := createRoute(destination)

	if !db.IsAddressRegistered(destination) {
		return nil, errors.New("Destination is not a registered address")
	}

	//Get an node pubkey associated with destination
	nodePubKey := db.GetAddressNode(destination)
	//Get IP address associated with node
	ipAddresses := GetNodeIPs(client, nodePubKey)

	log.Println(ipAddresses[0])

	//Start by connecting to the destination node
	ConnectToDestinationAuto(client, db, destination, route.token)

	//Add the first hop to the route and send forward the request through the network
	localHop, err := addHopToRoute(client, db, route)
	if err != nil {
		return nil, err
	}
	ForwardRoute(client, db, route, localHop)

	//Wait for the proble to reach the destination
	//and receive the route from the destination onode
	routeResponse := ReceiveRouteFromDestination(db, route.token)

	return routeResponse, nil
}

//GetRouteManual gets a route to a destination that is a public node
func GetRouteManual(client *lndwrapper.Lnd, db *DB, destination [4]byte, ipAddress string) (*Route, error) {

	route := createRoute(destination)

	if !db.IsAddressRegistered(destination) {
		return nil, errors.New("Destination is not a registered address")
	}

	//Start by connecting to the destination node
	ConnectToDestination(client, db, destination, ipAddress, route.token)

	//Add the first hop to the route and send forward the request through the network
	localHop, err := addHopToRoute(client, db, route)
	if err != nil {
		return nil, err
	}
	ForwardRoute(client, db, route, localHop)

	//Wait for the proble to reach the destination
	//and receive the route from the destination onode
	routeResponse := ReceiveRouteFromDestination(db, route.token)

	return routeResponse, nil
}

func addHopToRoute(client *lndwrapper.Lnd, db *DB, route *Route) ([4]byte, error) {

	routingEntry := db.getRoutingEntry(route.destination)

	if routingEntry == nil {
		return [4]byte{}, errors.New("No routing information for " + net.IP(route.destination[:]).String())
	}

	//Find the next hop channel balance
	hopPubKey := db.GetAddressNode(routingEntry.nextHop)
	hopPubKeyString := PubKeyArrayToString(hopPubKey)
	localChannels := GetLocalChannels(client)

	for _, localChannel := range localChannels {
		//Found the channel shared with the next hop neighbour
		if localChannel.RemotePubkey == hopPubKeyString {
			if len(route.hops) == 0 || localChannel.LocalBalance < route.capacity {
				route.capacity = localChannel.LocalBalance
			}
		}
	}

	//Append the next hop to the route
	route.hops = append(route.hops, routingEntry.nextHop)

	return routingEntry.nextHop, nil
}

//PrintRoute print a route using fmt
func PrintRoute(route *Route) {

	//Print the hops in the route
	fmt.Println("Route to", route.destination)

	//Print the max capacity available for this route
	fmt.Println("Maximum Capacity:", route.capacity)

	//Print every hop in the route
	for n, hop := range route.hops {
		fmt.Printf("Hop %v: %v\n", n, hop)
	}
}
