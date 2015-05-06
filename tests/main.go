package main

import (
	"fmt"                         // For outputting messages
	"github.com/Grayda/go-orvibo" // For controlling Orvibo stuff
	//	"github.com/davecgh/go-spew/spew" // For neatly outputting stuff
	"time" // For setInterval()
)

func main() {
	// These are our SetIntervals that run. To cancel one, simply send "<- true" to it (e.g. autoDiscover <- true)
	var autoDiscover, resubscribe chan bool

	ready, err := orvibo.Prepare() // You ready?
	if ready == true {             // Yep! Let's do this!
		// Because we'll never reach the end of the for loop (in theory),
		// we run SendEvent here.

		autoDiscover = setInterval(orvibo.Discover, time.Minute)
		resubscribe = setInterval(orvibo.Subscribe, time.Minute*3)
		orvibo.Discover() // Discover all sockets

		for { // Loop forever
			select { // This lets us do non-blocking channel reads. If we have a message, process it. If not, check for UDP data and loop
			case msg := <-orvibo.Events:
				switch msg.Name {
				case "existingsocketfound":
					fallthrough
				case "socketfound":
					fmt.Println("Socket found! MAC address is", msg.DeviceInfo.MACAddress)
					orvibo.Subscribe() // Subscribe to any unsubscribed sockets
					orvibo.Query()     // And query any unqueried sockets
				case "allonefound":
					fmt.Println("AllOne found! MAC address is", msg.DeviceInfo.MACAddress)
					orvibo.Subscribe() // Subscribe to any unsubscribed sockets
					orvibo.Query()     // And query any unqueried sockets

				case "subscribed":
					if msg.DeviceInfo.Subscribed == false {

						fmt.Println("Subscription successful!")

						orvibo.Devices[msg.DeviceInfo.MACAddress].Subscribed = true
						orvibo.Query()
						fmt.Println("Query called")

					}
					orvibo.Query()
				case "queried":

					if msg.DeviceInfo.Queried == false {
						orvibo.Devices[msg.DeviceInfo.MACAddress].Queried = true
						fmt.Println("Name of socket is:", msg.DeviceInfo.Name)
						orvibo.TestRF(msg.DeviceInfo.MACAddress)
					}

				case "statechanged":
					fmt.Println("State changed to:", msg.DeviceInfo.State)
				case "quit":
					autoDiscover <- true
					resubscribe <- true
				}
			default:
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
