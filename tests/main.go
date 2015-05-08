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
						orvibo.EmitIR("00000000980000000000000000008800f6227e1134023302260229023e02260233023302260229023e02270233023402260230023602900627026006630286063f029006270290061c029d06c1020e06270290061c02a406370227021a024c02260229023e029006270229023d029106270229023e0227021a029e063f029006270290061b025002220290061b024d02260290061b020000", msg.DeviceInfo.MACAddress)
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
