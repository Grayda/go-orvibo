package orvibo

// go-orvibo is a lightweight package that is used to control a variety of Orvibo products
// including the AllOne IR / 433mhz blaster and the S10 / S20 sockets

import (
	"encoding/hex" // For converting stuff to and from hex
	"errors"       // For crafting our own errors
	"fmt"          // For outputting stuff
	"math/rand"    // For the generation of random numbers
	"net"          // For networking stuff
	"strconv"
	"strings" // For string manipulation (indexOf etc.)

	"github.com/davecgh/go-spew/spew" // For neatly outputting stuff
)

// EventStruct is our equivalent to node.js's Emitters, of sorts.
// This basically passes back to our Event channel, info about what event was raised
// (e.g. Device, plus an event name) so we can act appropriately
type EventStruct struct {
	Name       string
	DeviceInfo *Device
}

// IRCode is a struct that describes our IR code. Name is a short name (e.g. "Power On") and Code is an IR hex string
type IRCode struct {
	ID   int
	Name string
	Code string
}

// RFSwitch contains info about RF switches. Access it through Device[macAdd].RFSwitches[switchID].State
type RFSwitch struct {
	State bool
}

// Device is info about the type of device that's been detected (socket, allone etc.)
type Device struct {
	ID            int          // The ID of our socket
	Name          string       // The name of our item
	DeviceType    int          // What type of device this is. See the const below for valid types
	IP            *net.UDPAddr // The IP address of our item
	MACAddress    string       // The MAC Address of our item. Necessary for controlling the S10 / S20 / AllOne
	Subscribed    bool         // Have we subscribed to this item yet? Doing so lets us control
	Queried       bool         // Have we queried this item for it's name and details yet?
	State         bool         // Is the item turned on or off? Will always be "false" for the AllOne, which doesn't do states, just IR & 433
	RFSwitches    map[string]RFSwitch
	LastIRMessage string // Not yet implemented.
	LastMessage   string // The last message to come through for this device

}

const (
	UNKNOWN = -1 + iota // UNKNOWN is obviously a device that isn't implemented or is unknown. iota means add 1 to the next const, so SOCKET = 0, ALLONE = 1 etc.
	SOCKET              // SOCKET is an S10 / S20 powerpoint socket
	ALLONE              // ALLONE is the AllOne IR blaster
	RF                  // RF switch. Not yet implemented
	KEPLER              // KEPLER is Orvibo's latest product, a timer / gas detector. Not yet implemented
)

// Events holds the events we'll be passing back to our calling code.
var Events = make(chan EventStruct, 1) // Events is our events channel which will notify calling code that we have an event happening
var Devices = make(map[string]*Device) // All the Devices we've discovered
var twenties = "202020202020"          // This is padding for the MAC Address. It appears often, so we define it here for brevity
var deviceCount int                    // How many items we've discovered
var conn *net.UDPConn                  // UDP Connection
// Our UDP connection

// ===============
// Exported Events
// ===============

// Prepare is the first function you should call. Gets our UDP connection ready
func Prepare() (bool, error) {

	_, err := getLocalIP() // Get our local IP. Used to test if there is a network connection issue
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
	passMessage("ready", &Device{})
	return true, nil
}

// Discover is a function that broadcasts 686400067161 over the network in order to find unpaired networks
func Discover() {
	// Wondering why we don't return anything? setInterval in our calling code can't handle returns
	_, err := broadcastMessage("686400067161")
	if err != nil {
		return
	}
	passMessage("discover", &Device{})
	return

}

// Subscribe loops over all the Devices we know about, and asks for control (subscription)
func Subscribe() {
	for k := range Devices { // Loop over all sockets we know about
		//if Devices[k].Subscribed == false { // If we haven't subscribed.
		// We send a message to each socket. reverseMAC takes a MAC address and reverses each pair (e.g. AC CF 23 becomes CA FC 32)
		SendMessage("6864001e636c"+Devices[k].MACAddress+twenties+reverseMAC(Devices[k].MACAddress)+twenties, Devices[k])
		//}
	}

	passMessage("subscribe", &Device{})
	// FIXME: success will be the last socket subscribed. If all fail but this one, will return true. If all succeed except last one, will return false
	return
}

// Query asks all the sockets we know about, for their names. Current state is sent on Subscription confirmation, not here
func Query() (bool, error) {
	var success bool
	var err error

	for k := range Devices { // Loop over all sockets we know about
		if Devices[k].Queried == false && Devices[k].Subscribed == true { // If we've subscribed but not queried..
			success, err = SendMessage("6864001D7274"+Devices[k].MACAddress+twenties+"0000000004000000000000", Devices[k])
		}
	}
	passMessage("query", &Device{})
	return success, err
}

// ListDevices spews out info about all the Devices we know about. It's great because it includes counts and other stuff
func ListDevices() {
	spew.Dump(&Devices)
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
		msg = nil
	}

	return success, err
}

// ToggleState finds out if the socket is on or off, then toggles it
func ToggleState(macAdd string) (bool, error) {
	if Devices[macAdd].State == true {
		return SetState(macAdd, false)
	}

	return SetState(macAdd, true)

}

// SetState sets the state of a socket, given its MAC address
func SetState(macAdd string, state bool) (bool, error) {
	if Devices[macAdd].DeviceType == SOCKET { // If it's a socket
		Devices[macAdd].State = state
		var statebit string
		if state == true {
			statebit = "01"
		} else {
			statebit = "00"
		}

		success, err := SendMessage("686400176463"+macAdd+twenties+"00000000"+statebit, Devices[macAdd])
		passMessage("stateset", Devices[macAdd])
		return success, err
	}
	return false, errors.New("Can't set state on a non-socket") // Naughty us, trying to set state on an AllOne!

}

// EmitIR emits IR from the AllOne. Takes a hex string
func EmitIR(IR string, macAdd string) {

	rnda := fmt.Sprintf("%02s", strconv.FormatInt(int64(rand.Intn(255)), 16)) // Gets a number between 0 and 255, makes it into a hex string, then pads it with zeros
	rndb := fmt.Sprintf("%02s", strconv.FormatInt(int64(rand.Intn(255)), 16)) // Gets a number between 0 and 255, makes it into a hex string, then pads it with zeros
	var irlen = fmt.Sprintf("%04s", strconv.FormatInt(int64(len(IR)/2), 16))
	irlen = irlen[2:4] + irlen[0:2]
	var packet = "6864" + "0000" + "6963" + "accfdeadbeef" + twenties + "65000000" + rnda + rndb + irlen + IR
	var packetlen = fmt.Sprintf("%04s", strconv.FormatInt(int64(len(packet)/2), 16))

	// 6864 irlen 6963 mac 202020202020 65 00 00 00 rnda rndb, len of IR, IR
	// this.hex2ba(hosts[index].macaddress), twenties, ['0x65', '0x00', '0x00', '0x00'], randomBitA, randomBitB, this.hex2ba(irLength), this.hex2ba(ir));
	if macAdd == "ALL" {
		for _, allones := range Devices {
			if allones.DeviceType == ALLONE {
				packet = "6864" + packetlen + "6963" + allones.MACAddress + twenties + "65000000" + rnda + rndb + irlen + IR

				SendMessage(packet, allones)
			}
		}
	} else {
		if Devices[macAdd].DeviceType == ALLONE {
			packet = "6864" + packetlen + "6963" + macAdd + twenties + "65000000" + rnda + rndb + irlen + IR
			SendMessage(packet, Devices[macAdd])
		}
	}
}

func EmitRF(RF string, macAdd string) {

	rnda := fmt.Sprintf("%02s", strconv.FormatInt(int64(rand.Intn(255)), 16)) // Gets a number between 0 and 255, makes it into a hex string, then pads it with zeros
	rndb := fmt.Sprintf("%02s", strconv.FormatInt(int64(rand.Intn(255)), 16)) // Gets a number between 0 and 255, makes it into a hex string, then pads it with zeros
	var packet = "6864" + "0000" + "6463" + "accfdeadbeef" + twenties + "3ef5ee0b" + rnda + rndb + RF
	var packetlen = fmt.Sprintf("%04s", strconv.FormatInt(int64(len(packet)/2), 16))

	// 6864 irlen 6963 mac 202020202020 65 00 00 00 rnda rndb, len of IR, IR
	// this.hex2ba(hosts[index].macaddress), twenties, ['0x65', '0x00', '0x00', '0x00'], randomBitA, randomBitB, this.hex2ba(irLength), this.hex2ba(ir));
	if macAdd == "ALL" {
		for _, allones := range Devices {
			if allones.DeviceType == ALLONE {
				packet = "6864" + packetlen + "6463" + allones.MACAddress + twenties + "3ef5ee0b" + rnda + rndb + RF
				SendMessage(packet, allones)
			}
		}
	} else {
		if Devices[macAdd].DeviceType == ALLONE {
			packet = "6864" + packetlen + "6463" + macAdd + twenties + "3ef5ee0b" + rnda + rndb + RF
			SendMessage(packet, Devices[macAdd])
		}
	}
}

func EnterLearningMode(macAdd string) {
	if macAdd == "ALL" {
		for _, allones := range Devices {
			if allones.DeviceType == ALLONE {
				SendMessage("686400186c73"+allones.MACAddress+twenties+"010000000000", allones)
				passMessage("irlearnmode", allones)
			}
		}
	} else {
		if Devices[macAdd].DeviceType == ALLONE {
			SendMessage("686400186c73"+macAdd+twenties+"010000000000", Devices[macAdd])
			passMessage("irlearnmode", Devices[macAdd])
		}
	}
}

func EnterRFLearningMode(macAdd string) {
	SendMessage("646400187266"+macAdd+twenties+"010000000000", Devices[macAdd])
	passMessage("rflearnmode", Devices[macAdd])
}

// SendMessage is the heart of our library. Sends UDP messages to specified IP addresses
func SendMessage(msg string, device *Device) (bool, error) {

	// Turn this hex string into bytes for sending
	buf, _ := hex.DecodeString(msg)

	// Resolve our address, ready for sending data
	udpAddr, resolveErr := net.ResolveUDPAddr("udp4", device.IP.String())
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

	passMessage("sendmessage", device)
	return true, nil
}

// ==================
// Internal functions
// ==================

// handleMessage parses a message found by CheckForMessages
func handleMessage(message string, addr *net.UDPAddr) (bool, error) {

	if len(message) == 0 { // Blank message? Don't try and parse it!
		return false, errors.New("Blank message")
	}

	// If this is a broadcast message
	if message == "686400067161" {
		return true, nil
	}

	commandID := message[8:12]                  // What command we've received back
	macStart := strings.Index(message, "accf")  // Find where our MAC Address starts
	macAdd := message[macStart:(macStart + 12)] // The MAC address of the socket responding

	switch commandID {
	case "7161": // We've had a response to our broadcast message

		_, exists := Devices[macAdd] // Check to see if we've already got macAdd in our array

		if strings.Index(message, "49524430") > 0 { // Contains SOC0? It's a socket!
			if exists == false { // We haven't got it in our Devices array?
				deviceCount++ // Add one to the deviceCount
				Devices[macAdd] = &Device{
					ID:            deviceCount,
					Name:          "", // No name yet
					DeviceType:    ALLONE,
					IP:            addr,
					MACAddress:    macAdd,
					Subscribed:    false,
					Queried:       false,
					State:         false,
					RFSwitches:    make(map[string]RFSwitch), // Lightswitches
					LastIRMessage: "",                        // The last IR message we've received
					LastMessage:   message,                   // The last message we received
				}

				passMessage("allonefound", Devices[macAdd]) // Let our calling code know
			} else {
				Devices[macAdd].LastMessage = message // Set our LastMessage
				passMessage("existingallonefound", Devices[macAdd])
			}

		} else if strings.Index(message, "534f4330") > 0 { // Contains IRD0? It's an IR blaster!
			if exists == false { // If we don't have this device in our list already
				deviceCount++ // Add one to the deviceCount
				Devices[macAdd] = &Device{
					ID:            deviceCount,
					Name:          "",
					DeviceType:    SOCKET,
					IP:            addr,
					MACAddress:    macAdd,
					Subscribed:    false,
					Queried:       false,
					State:         false,
					RFSwitches:    make(map[string]RFSwitch),
					LastIRMessage: "",
					LastMessage:   message,
				}

				lastBit := message[(len(message) - 1):] // Get the last bit from our message. 0 or 1 for off or on
				if lastBit == "0" {
					Devices[macAdd].State = false
				} else {
					Devices[macAdd].State = true
				}

				passMessage("socketfound", Devices[macAdd])
			} else {
				Devices[macAdd].LastMessage = message // Set our LastMessage
				passMessage("existingsocketfound", Devices[macAdd])
			}
		} else {
			Devices[macAdd].LastMessage = message // Set our LastMessage
			passMessage("unknownhardwarefound", Devices[macAdd])
		}

	case "636c": // We've had confirmation of subscription

		// Sometimes we receive messages for sockets we don't know about. The WiWo
		// app does this sometimes, as it sends messages to all AllOnes it knows about,
		// regardless of whether or not they're active on the network. So we
		// check to see if the socket that needs updating exists in our list. If it doesn't,
		// we return false.
		if exists(macAdd) == false {
			return false, nil
		}

		lastBit := message[(len(message) - 1):] // Get the last bit from our message. 0 or 1 for off or on
		if lastBit == "1" {
			Devices[macAdd].State = true
		} else {
			Devices[macAdd].State = false
		}

		Devices[macAdd].LastMessage = message // Set our LastMessage
		passMessage("subscribed", Devices[macAdd])

	case "6463": // Someone's pressed an RF switch.
		passMessage("rfswitch", Devices[macAdd])

	case "7274": // We've queried our socket, this is the data back

		// Our name starts after the fourth 202020202020, or 140 bytes in
		strName := strings.TrimRight(message[140:172], "")

		// And our name is 32 bytes long.
		strDecName, _ := hex.DecodeString(strName[0:32])

		// If no name has been set, we get 32 bytes of F back, so
		// we create a generic name so our socket name won't be spaces
		if strName == "20202020202020202020202020202020" || strName == "ffffffffffffffffffffffffffffffff" {
			if Devices[macAdd].DeviceType == SOCKET {
				Devices[macAdd].Name = "Socket " + macAdd
			} else {
				Devices[macAdd].Name = "AllOne " + macAdd
			}

		} else { // If a name WAS set
			Devices[macAdd].Name = string(strDecName) // Convert back to text and assign
		}

		Devices[macAdd].LastMessage = message // Set our LastMessage
		passMessage("queried", Devices[macAdd])

	case "7366": // Confirmation of state change

		lastBit := message[(len(message) - 1):] // Get the last bit from our message. 0 or 1 for off or on
		if lastBit == "0" {
			Devices[macAdd].State = false
		} else {
			Devices[macAdd].State = true
		}

		Devices[macAdd].LastMessage = message // Set our LastMessage
		passMessage("statechanged", Devices[macAdd])

	case "6469": // We've pressed the button on the top of our AllOne
		Devices[macAdd].LastMessage = message // Set our LastMessage
		passMessage("buttonpress", Devices[macAdd])
	case "6c73": // We've had an IR code back after learning mode
		// 686400186c73accf232a5ffa202020202020000000000000
		if len(message) >= 52 {
			Devices[macAdd].LastIRMessage = message[52:]
			Devices[macAdd].LastMessage = message // Set our LastMessage
			passMessage("ircode", Devices[macAdd])
		}
	default: // No message? Return true
		return true, nil
	}

	return true, nil
}

// Do we have macAdd in our Devices list?
func exists(macAdd string) bool {
	_, exists := Devices[macAdd]
	return exists
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
func passMessage(message string, device *Device) bool {

	select {
	case Events <- EventStruct{message, device}:

	default:
	}

	return true
}

// broadcastMessage is another core part of our code. It lets us broadcast a message to the whole network.
// It's essentially SendMessage with a IPv4 Broadcast address
func broadcastMessage(msg string) (bool, error) {

	udpAddr, err := net.ResolveUDPAddr("udp4", net.IPv4bcast.String()+":10000")
	SendMessage(msg, &Device{IP: udpAddr})
	if err != nil {
		return false, err
	}
	passMessage("broadcast", &Device{})
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
