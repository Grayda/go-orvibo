package main

import (
	"fmt"                         // For outputting messages
	"github.com/Grayda/go-orvibo" // For controlling Orvibo stuff
	"time"                        // For setInterval()
)

func main() {
	ready, err := orvibo.Prepare() // You ready?
	if ready == true {             // Yep! Let's do this!
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
				case "subscribed":
					orvibo.Query()
					orvibo.Subscribe()
				case "queried":
					fmt.Println("We've queried. Name is:", msg.DeviceInfo.Name)
					orvibo.SetState(msg.DeviceInfo.MACAddress, true)
					time.Sleep(time.Second)
					orvibo.SetState(msg.DeviceInfo.MACAddress, false)
				case "statechanged":
					fmt.Println("State changed to", msg.DeviceInfo.State)
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
