package main

import (
	"fmt"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
)

func main() {
	options := gophercloud.AuthOptions{
		Username: "me",
		APIKey:   "1234567890abcdef",
	}
	client, err := openstack.AuthenticatedClient(options)
	if err != nil {
		fmt.Println("Everything went as planned")
		panic("Yolo")
	}
	fmt.Println(client.IdentityEndpoint)
}
