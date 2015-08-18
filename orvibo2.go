package orvibo2

import (
	"encoding/hex"
	"net"
)

// EventStruct is our equivalent to node.js's Emitters, of sorts.
// This basically passes back to our Event channel, info about what event was raised
// (e.g. Device, plus an event name) so we can act appropriately
type EventStruct struct {
	Name       string
	DeviceInfo *Device
}

// A list of supported products
const (
	UNKNOWN = -1 + iota // UNKNOWN is obviously a device that isn't implemented or is unknown. iota means add 1 to the next const, so SOCKET = 0, ALLONE = 1 etc.
	SOCKET              // SOCKET is an S10 / S20 powerpoint socket
	ALLONE              // ALLONE is the AllOne IR blaster
	RF                  // RF switch. Not yet implemented
	KEPLER              // KEPLER is Orvibo's latest product, a timer / gas detector. Not yet implemented
)

// AllOne is Orvibo's IR and 433mhz blaster.
type AllOne struct {
	Name          string              // The name of our item
	DeviceType    int                 // What type of device this is. See the const below for valid types
	IP            *net.UDPAddr        // The IP address of our item
	MACAddress    string              // The MAC Address of our item. Necessary for controlling the S10 / S20 / AllOne
	Subscribed    bool                // Have we subscribed to this item yet? Doing so lets us control
	Queried       bool                // Have we queried this item for it's name and details yet?
	RFSwitches    map[string]RFSwitch // What switches are attached to this device?
	LastIRMessage string              // Not yet implemented.
	LastMessage   string              // The last message to come through for this device
}

// Socket refers to the S10 and S20 WiFi enabled power sockets
type Socket struct {
	Name        string       // The name of our item
	DeviceType  int          // What type of device this is. See the const below for valid types
	IP          *net.UDPAddr // The IP address of our item
	MACAddress  string       // The MAC Address of our item. Necessary for controlling the S10 / S20 / AllOne
	Subscribed  bool         // Have we subscribed to this item yet? Doing so lets us control
	Queried     bool         // Have we queried this item for it's name and details yet?
	State       bool         // Is the item turned on or off? Will always be "false" for the AllOne, which doesn't do states, just IR & 433
	LastMessage string       // The last message to come through for this device
}

// RFSwitch is Orvibo's RF (433mhz) switch. It's not WiFi enabled, so it receives signals from an AllOne
type RFSwitch struct {
	Name        string // The name of our item
	DeviceType  int    // What type of device this is. See the const below for valid types
	State       bool   // Is the item turned on or off? Will always be "false" for the AllOne, which doesn't do states, just IR & 433
	AllOne      AllOne // Which AllOne is this switch attached to?
	LastMessage string // The last message to come through for this device
}

// Kepler is Orvibo's newest product. It's a Gas & CO2 detector that has a timer built in
type Kepler struct {
	Name       string       // The name of our item
	DeviceType int          // What type of device this is. See the const below for valid types
	IP         *net.UDPAddr // The IP address of our item
	MACAddress string       // The MAC Address of our item. Necessary for controlling the S10 / S20 / AllOne
	Subscribed bool         // Have we subscribed to this item yet? Doing so lets us control
	Queried    bool         // Have we queried this item for it's name and details yet?
	CO2        int          // The amount of CO2 in the air
	Gas        int          // The amount of gas in the air
}

// All Orvibo packets start with this sequence, which is "hd" in hex
var magicWord = "6864"

// A list of devices we know about
var Devices map[string]interface{}

// Start listens on UDP port 10000 for incoming messages.
func Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp4", ":10000") // Get our address ready for listening
	if err != nil {
		return err
	}

	conn, err = net.ListenUDP("udp", udpAddr) // Now we listen on the address we just resolved
	if listenErr != nil {
		return listenErr
	}

	// Hand a message back to our calling code. Because it's not about a particular device, we just pass back an empty AllOne
	passMessage("ready", &AllOne{})
	return nil
}

// Discover all Orvibo devices
func Discover() {
	// magicWord + packet length + command ID ("qa", which means search for sockets where MAC is unknown)
	err := broadcastMessage(magicWord + "0006" + "7161")
	if err != nil {
		return
	}

	passMessage("discover", &Device{})
	return
}

// SendMessage is the heart of our library. Sends UDP messages to specified IP addresses
func SendMessage(msg string, device interface{}) error {
	// Turn this hex string into bytes for sending
	buf, _ := hex.DecodeString(msg)

	// Resolve our address, ready for sending data
	udpAddr, err := net.ResolveUDPAddr("udp4", device.IP.String())
	if err != nil {
		return err
	}

	// Actually write the data and send it off
	// _ lets us ignore "declared but not used" errors. If we replace _ with n (number of bytes),
	// We'd have to use n somewhere (e.g. fmt.Println(n, "bytes received")), but _ lets us ignore that
	_, err = conn.WriteToUDP(buf, udpAddr)
	// If we've got an error
	if err != nil {
		return err
	}

	passMessage("sendmessage", device)
	return nil
}

// Sends a message to the whole network via UDP
func broadcastMessage(msg string) error {
	udpAddr, err := net.ResolveUDPAddr("udp4", net.IPv4bcast.String()+":10000")
	// We create a temporary AllOne with an IP address of 255.255.255.255 so SendMessage will work without modification
	SendMessage(msg, &AllOne{IP: udpAddr})
	if err != nil {
		return err
	}

	// Info for our calling code
	passMessage("broadcast", &AllOne{})
	return nil
}
