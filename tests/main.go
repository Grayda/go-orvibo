package main

import (
	"fmt" // For outputting messages

	"time" // For setInterval()

	"github.com/Grayda/go-orvibo"     // For controlling Orvibo stuff
	"github.com/davecgh/go-spew/spew" // For neatly outputting stuff
)

func main() {
	// These are our SetIntervals that run. To cancel one, simply send "<- true" to it (e.g. autoDiscover <- true)
	var autoDiscover, resubscribe chan bool

	ready, err := orvibo.Prepare() // You ready?
	if ready == true {             // Yep! Let's do this!
		// Look for new devices every minute
		autoDiscover = setInterval(orvibo.Discover, time.Minute)
		// Resubscription should happen every 5 minutes, but we make it 3, just to be on the safe side
		resubscribe = setInterval(orvibo.Subscribe, time.Minute*3)
		orvibo.Discover() // Discover all sockets

		for { // Loop forever
			select { // This lets us do non-blocking channel reads. If we have a message, process it. If not, check for UDP data and loop
			case msg := <-orvibo.Events:
				switch msg.Name {
				case "ready": // We're set up and ready to go
					fmt.Println("UDP connection ready")
				case "discover": // We're discovering devices
					fmt.Println("Discovering devices")
				case "subscribe": // We're subscribing to all known devices
					fmt.Println("Subscribing to devices")
				case "query": // We're querying any unqueried devices
					fmt.Println("Querying any unqueried devices")
				case "stateset": // Raised by orvibo.SetState(). Let us know that we're attempting a state set
					fmt.Println("Setting state to", msg.DeviceInfo.State)
				case "irlearnmode": // Entering IR learning mode. I don't think there's an RF learning mode though.
					fmt.Println("Entering learning mode for", msg.DeviceInfo.Name)
				case "unknownhardwarefound": // We've found a device that follows the Orvibo "protocol", but we don't know what it is!
					fmt.Println("Unknown hardware detected! Whole message is:", msg.DeviceInfo.LastMessage)
				case "buttonpress": // Someone's pressed the button on top of our AllOne
					fmt.Println("Button on", msg.DeviceInfo.Name, "has been pressed")
				case "ircode": // We've learned an IR code, and this is what the code is
					fmt.Println("IR code found!", msg.DeviceInfo.LastIRMessage)
				case "existingsocketfound": // We've found a socket that we already know about. Can be used for resubscription purposes?
					fallthrough
				case "existingallonefound":
					fallthrough
				case "socketfound": // We've found a socket!
					fmt.Println("Socket found! MAC address is", msg.DeviceInfo.MACAddress)
					orvibo.Subscribe() // Subscribe to any unsubscribed sockets
					orvibo.Query()     // And query any unqueried sockets
				case "allonefound": // We've found an AllOne!
					fmt.Println("AllOne found! MAC address is", msg.DeviceInfo.MACAddress)
					orvibo.Subscribe() // Subscribe to any unsubscribed sockets
					orvibo.Query()     // And query any unqueried sockets
				case "subscribed": // We've subscribed to a device, and it's been successful
					if msg.DeviceInfo.Subscribed == false {
						fmt.Println("Subscription successful!")
						orvibo.Devices[msg.DeviceInfo.MACAddress].Subscribed = true
						orvibo.Query()
					}
					orvibo.Query()
				case "queried": // We've successfully queried a device and can now access its reported name and so forth
					if msg.DeviceInfo.Queried == false {
						orvibo.Devices[msg.DeviceInfo.MACAddress].Queried = true
						// spew.Dump(msg.DeviceInfo)
					}

					orvibo.EmitRF(true, "2b00daaeeb", msg.DeviceInfo.MACAddress)

				case "rfswitch": // Someone's toggled an RF switch. Still in alpha stage
					fmt.Println("RF switch pressed")
					spew.Dump(msg.DeviceInfo.RFSwitches)
				case "statechanged": // Something external has triggered a state change, or we've got confirmation of a state change
					fmt.Println("State of", msg.DeviceInfo.Name, "changed to:", msg.DeviceInfo.State)
				case "quit": // Not used.
					autoDiscover <- true
					resubscribe <- true
				}
			default: // No messages? Check for new messages
				orvibo.CheckForMessages()
			}

		}
	} else {
		fmt.Println("Error:", err)

	}

}

func setInterval(what func(), delay time.Duration) chan bool {
	stop := make(chan bool)

	go func() {
		for {
			what()
			select {
			case <-time.After(delay):
			case <-stop:
				return
			}
		}
	}()

	return stop
}
