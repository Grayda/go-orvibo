package orvibo

// go-orvibo is a lightweight package that is used to control a variety of Orvibo products
// including the AllOne IR / 433mhz blaster and the S10 / S20 sockets

import (
	"encoding/hex"                    // For converting stuff to and from hex
	"errors"                          // For crafting our own errors
	"fmt"                             // For outputting stuff
	"github.com/davecgh/go-spew/spew" // For neatly outputting stuff
	"net"                             // For networking stuff
	"strings"                         // For string manipulation (indexOf etc.)
)

// EventStruct is our equivalent to node.js's Emitters, of sorts.
// This basically passes back to our Event channel, info about what event was raised
// (e.g. Device, plus an event name) so we can act appropriately
type EventStruct struct {
	Name       string
	DeviceInfo Device
}

// Device is info about the type of device that's been detected (socket, allone etc.)
type Device struct {
	Name              string       // The name of our item
	Type              int          // What type of device this is. See the const below for valid types
	IP                *net.UDPAddr // The IP address of our item
	MACAddress        string       // The MAC Address of our item. Necessary for controlling the S10 / S20 / AllOne
	Subscribed        bool         // Have we subscribed to this item yet? Doing so lets us control
	Queried           bool         // Have we queried this item for it's name and details yet?
	State             bool         // Is the item turned on or off? Will always be "false" for the AllOne, which doesn't do states, just IR & 433
	LastIRMessage     string       // The last IR message to come in, if our device is an AllOne. It's always blank, otherwise
	Last433mhzMessage string       // Not yet implemented.
	LastMessage       string       // The last message to come through for this device
}

const (
	UNKNOWN = -1 + iota // UNKNOWN is obviously a device that isn't implemented or is unknown. iota means add 1 to the next const, so SOCKET = 0, ALLONE = 1 etc.
	SOCKET              // SOCKET is an S10 / S20 powerpoint socket
	ALLONE              // ALLONE is the AllOne IR blaster
	RF                  // RF switch. Not yet implemented
	KEPLER              // KEPLER is Orvibo's latest product, a timer / gas detector. Not yet implemented
)

var conn *net.UDPConn // UDP Connection

var Events = make(chan EventStruct, 1) // Events is our events channel which will notify calling code that we have an event happening
var devices = make(map[string]*Device) // All the devices we've discovered
var twenties = "202020202020"          // This is padding for the MAC Address. It appears often, so we define it here for brevity
// Our UDP connection

// ===============
// Exported Events
// ===============

// Prepare is the first function you should call. Gets our UDP connection ready
func Prepare() (bool, error) {
	_, err := getLocalIP() // Get our local IP. Not actually used in this func, but is more of a failsafe
	if err != nil {        // Error? Return false
		return false, err
	}

	udpAddr, resolveErr := net.ResolveUDPAddr("udp4", ":10000") // Get our address ready for listening
	if resolveErr != nil {
		return false, resolveErr
	}

	var listenErr error
	conn, listenErr = net.ListenUDP("udp", udpAddr) // Now we listen on the address we just resolved
	if listenErr != nil {
		return false, listenErr
	}

	return true, nil
}

// Discover is a function that broadcasts 686400067161 over the network in order to find unpaired networks
func Discover() {
	_, err := broadcastMessage("686400067161")
	if err != nil {
		return
	}

	return

}

// Subscribe loops over all the devices we know about, and asks for control (subscription)
func Subscribe() {
	for k := range devices { // Loop over all sockets we know about
		if devices[k].Subscribed == false { // If we haven't subscribed.
			// We send a message to each socket. reverseMAC takes a MAC address and reverses each pair (e.g. AC CF 23 becomes CA FC 32)
			sendMessage("6864001e636c"+devices[k].MACAddress+twenties+reverseMAC(devices[k].MACAddress)+twenties, devices[k].IP)
		}
	}
	// FIXME: success will be the last socket subscribed. If all fail but this one, will return true. If all succeed except last one, will return false
	return
}

// Query asks all the sockets we know about, for their names. Current state is sent on Subscription confirmation, not here
func Query() (bool, error) {
	var success bool
	var err error

	for k := range devices { // Loop over all sockets we know about
		if devices[k].Queried == false && devices[k].Subscribed == true { // If we've subscribed but not queried..
			success, err = sendMessage("6864001D7274"+devices[k].MACAddress+twenties+"0000000004000000000000", devices[k].IP)
		}
	}

	return success, err
}

// ListDevices spews out info about all the devices we know about. It's great because it includes counts and other stuff
func ListDevices() {
	spew.Dump(&devices)
}

// CheckForMessages does what it says on the tin -- checks for incoming UDP messages
func CheckForMessages() (bool, error) { // Now we're checking for messages

	var msg []byte     // Holds the incoming message
	var buf [1024]byte // We want to get 1024 bytes of messages (is this enough? Need to check!)

	var success bool
	var err error

	n, addr, _ := conn.ReadFromUDP(buf[0:]) // Read 1024 bytes from the buffer
	ip, _ := getLocalIP()                   // Get our local IP
	if n > 0 && addr.IP.String() != ip {    // If we've got more than 0 bytes and it's not from us

		msg = buf[0:n]                                              // n is how many bytes we grabbed from UDP
		success, err = handleMessage(hex.EncodeToString(msg), addr) // Hand it off to our handleMessage func. We pass on the message and the address (for replying to messages)
		msg = nil                                                   // Clear out our msg property so we don't run handleMessage on old data
	} else {
		fmt.Println("From Us:", msg)
		msg = nil
	}

	return success, err
}

// ToggleState finds out if the socket is on or off, then toggles it
func ToggleState(macAdd string) (bool, error) {
	if devices[macAdd].State == true {
		return SetState(macAdd, false)
	}

	return SetState(macAdd, true)

}

// SetState sets the state of a socket, given its MAC address
func SetState(macAdd string, state bool) (bool, error) {
	if devices[macAdd].Type == SOCKET { // If it's a socket
		devices[macAdd].State = state
		var statebit string
		if state == true {
			statebit = "01"
		} else {
			statebit = "00"
		}

		success, err := sendMessage("686400176463"+macAdd+twenties+"00000000"+statebit, devices[macAdd].IP)
		passMessage("stateset", *devices[macAdd])
		return success, err
	}
	return false, errors.New("Can't set state on a non-socket") // Naughty us, trying to set state on an AllOne!

}

// ==================
// Internal functions
// ==================

// handleMessage parses a message found by CheckForMessages
func handleMessage(message string, addr *net.UDPAddr) (bool, error) {
	fmt.Println("Message from", addr.IP)
	if len(message) == 0 { // Blank message? Don't try and parse it!
		return false, errors.New("Blank message")
	}

	if message == "686400067161" {
		return true, nil
	}
	commandID := message[8:12] // What command we've received back

	macStart := strings.Index(message, "accf")  // Find where our MAC Address starts
	macAdd := message[macStart:(macStart + 12)] // The MAC address of the socket responding

	switch commandID {
	case "7161": // We've had a response to our broadcast message

		_, ok := devices[macAdd] // Check to see if we've already got macAdd in our array

		if strings.Index(message, "4952443030") > 0 { // Contains SOC00? It's a socket!
			if ok == false { // We haven't got it in our devices array?
				devices[macAdd] = &Device{"", ALLONE, addr, macAdd, false, false, false, "", "", ""} // Add the device
				passMessage("allonefound", *devices[macAdd])                                         // Let our calling code know
			} else {
				passMessage("existingallonefound", *devices[macAdd])
			}

		} else if strings.Index(message, "534f433030") > 0 { // Contains IRD00? It's an IR blaster!
			if ok == false {
				devices[macAdd] = &Device{"", SOCKET, addr, macAdd, false, false, false, "", "", ""} // Add the device
				passMessage("socketfound", *devices[macAdd])
			} else {
				passMessage("existingsocketfound", *devices[macAdd])
			}
		} else {
			passMessage("unknownhardwarefound", *devices[macAdd])
		}

	case "636c": // We've had confirmation of subscription
		devices[macAdd].Subscribed = true
		ListDevices()
		passMessage("subscribed", *devices[macAdd])

	case "7274": // We've queried our socket, this is the data back
		// Our name starts after the fourth 202020202020, or 140 bytes in
		strName := message[140:172]
		strName = strings.TrimRight(strName, "") // Get rid of the spaces at the end

		// And our name is 32 bytes long.
		strDecName, _ := hex.DecodeString(strName[0:32])

		// If no name has been set, we get 32 bytes of F back, so
		// we create a generic name so our socket name won't be spaces
		if strName == "20202020202020202020202020202020" {
			devices[macAdd].Name = "Socket " + macAdd
		} else { // If a name WAS set
			devices[macAdd].Name = string(strDecName) // Convert back to text and assign
		}

		passMessage("queried", *devices[macAdd])

	case "7366": // Confirmation of state change

		lastBit := message[(len(message) - 1):] // Get the last bit from our message. 0 or 1 for off or on
		if lastBit == "0" {
			devices[macAdd].State = false
		} else {
			devices[macAdd].State = true
		}

		passMessage("statechanged", *devices[macAdd])

	default: // No message? Return true
		return true, nil
	}
	devices[macAdd].LastMessage = message // Set our LastMessage
	return true, nil
}

// Gets our current IP address. This is used so we can ignore messages from ourselves
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	ifaces, _ := net.Interfaces()
	// handle err
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		// handle err
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPAddr:
				return v.IP.String(), nil
			}

		}
	}

	return "", errors.New("Unable to find IP address. Ensure you're connected to a network")
}

// passMessage adds items to our Events channel so the calling code can be informed
// It's non-blocking or whatever.
func passMessage(message string, device Device) bool {

	select {
	case Events <- EventStruct{message, device}:

	default:
	}

	return true
}

// sendMessage is the heart of our library. Sends UDP messages to specified IP addresses
func sendMessage(msg string, ip *net.UDPAddr) (bool, error) {
	// Turn this hex string into bytes for sending
	buf, _ := hex.DecodeString(msg)

	// Resolve our address, ready for sending data
	udpAddr, resolveErr := net.ResolveUDPAddr("udp4", ip.String())
	if resolveErr != nil {
		return false, resolveErr
	}

	// Actually write the data and send it off
	// _ lets us ignore "declared but not used" errors. If we replace _ with n (number of bytes),
	// We'd have to use n somewhere (e.g. fmt.Println(n, "bytes received")), but _ lets us ignore that
	_, sendErr := conn.WriteToUDP(buf, udpAddr)
	// If we've got an error
	if sendErr != nil {
		return false, sendErr
	}

	return true, nil
}

// broadcastMessage is another core part of our code. It lets us broadcast a message to the whole network.
// It's essentially sendMessage with a IPv4 Broadcast address
func broadcastMessage(msg string) (bool, error) {

	udpAddr, err := net.ResolveUDPAddr("udp4", net.IPv4bcast.String()+":10000")
	sendMessage(msg, udpAddr)
	if err != nil {
		return false, err
	}

	return true, nil
}

// Via http://stackoverflow.com/questions/19239449/how-do-i-reverse-an-array-in-go
// Splits up a hex string into bytes then reverses the bytes
func reverseMAC(mac string) string {
	s, _ := hex.DecodeString(mac)
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return hex.EncodeToString(s)
}
