package main

import (
	"fmt"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/spf13/viper"
)

type credentials struct {
	KeystoneURI        string `yaml:keystone_uri`
	SwiftConsoTenant   string `yaml:swift_conso_tenant`
	SwiftConsoUser     string `yaml:swift_conso_user`
	SwiftConsoPassword string `yaml:swift_conso_password`
}

func buildAuthOptions() (gophercloud.AuthOptions, error) {
	var creds credentials
	if err := viper.UnmarshalKey("credentials", &creds); err != nil {
		return gophercloud.AuthOptions{}, err
	}

	ao := gophercloud.AuthOptions{
		Username:         creds.SwiftConsoUser,
		TenantName:       creds.SwiftConsoTenant,
		Password:         creds.SwiftConsoPassword,
		IdentityEndpoint: creds.KeystoneURI,
	}
	return ao, nil
}

func main() {

	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")                                        // name of config file (without extension)
	viper.AddConfigPath("/home/felix/gbprojects/swift-consometer/etc/swift") // path to look for the config file in
	err := viper.ReadInConfig()                                              // Find and read the config file
	if err != nil {                                                          // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	options, err := buildAuthOptions()
	if err != nil {
		panic(fmt.Errorf("Fatal failed to read credentials : %s \n", err))
	}
	fmt.Println(viper.AllSettings())

	client, err := openstack.AuthenticatedClient(options)
	if err != nil {
		fmt.Println("Everything went as planned")
		panic("Yolo")
	}
	fmt.Println(client.IdentityEndpoint)
}
