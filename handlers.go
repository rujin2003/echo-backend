package main

import (
	"fmt"
)

func handleBatteryUpdate(event Event, c *Client) error {

	fmt.Printf("Battery update received from")
	return nil
}

func handleDeviceInfo(event Event, c *Client) error {
	fmt.Printf("Device info update is coming")
	return nil
}
func handleDownloads(event Event, c *Client) error {
	fmt.Printf("Downloads")

	return nil
}

func handleActions(event Event, c *Client) error {
	fmt.Printf("Actions")

	return nil
}
func handleSubscribe(event Event, c *Client) error {
	fmt.Printf("subscribe")

	return nil
}
