package main

import (
	"flag"
	"fmt"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/openstack/identity/v2/tenants"
	"github.com/rackspace/gophercloud/pagination"
	"github.com/spf13/viper"
	"strings"
)

/*
type credentials struct {
	KeystoneURI        string
	SwiftConsoTenant   string
	SwiftConsoUser     string
	SwiftConsoPassword string
}

func buildAuthOptions() (gophercloud.AuthOptions, error) {
	var creds credentials
	if err := viper.UnmarshalKey("credentials", &creds); err != nil {
		//return gophercloud.AuthOptions{}, err
		panic(fmt.Errorf("Unable to decode into struct, %s \n", err))
	}

	ao := gophercloud.AuthOptions{
		Username:         creds.SwiftConsoUser,
		TenantName:       creds.SwiftConsoTenant,
		Password:         creds.SwiftConsoPassword,
		IdentityEndpoint: creds.KeystoneURI,
	}
	return ao, nil
}
*/

func getTenants(client *gophercloud.ServiceClient) []tenants.Tenant {
	var list []tenants.Tenant
	pager := tenants.List(client, nil)
	pager.EachPage(func(page pagination.Page) (bool, error) {
		tenantList, err := tenants.ExtractTenants(page)
		list = append(tenantList)
		if err != nil {
			panic(fmt.Errorf("Error processing pager: %s \n", err))
		}
		return true, nil
	})
	return list
}

func main() {
	ConfigPath := flag.String("config", "./etc/swift", "Path of the configuration file directory.")
	flag.Parse()
	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer") // name of config file (without extension)
	viper.AddConfigPath(*ConfigPath)  // path to look for the config file in
	err := viper.ReadInConfig()       // Find and read the config file
	if err != nil {                   // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	regionName := viper.GetString("os_region_name")

	//	opts, err := buildAuthOptions()
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: viper.GetString("credentials.keystone_uri"),
		Username:         viper.GetString("credentials.swift_conso_user"),
		Password:         viper.GetString("credentials.swift_conso_password"),
		TenantName:       viper.GetString("credentials.swift_conso_tenant"),
	}
	if err != nil {
		panic(fmt.Errorf("Fatal failed to read credentials : %s \n", err))
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		panic(fmt.Errorf("Fatal failed te create provider file : %s \n", err))
	}

	idClient := openstack.NewIdentityV2(provider)
	if err != nil {
		panic(fmt.Errorf("Fatal failed to create identity client : %s \n", err))
	}
	tenantList := getTenants(idClient)
	objectStoreURL, err := provider.EndpointLocator(gophercloud.EndpointOpts{Type: "object-store", Region: regionName, Availability: gophercloud.AvailabilityAdmin})
	if err != nil {
		panic(fmt.Errorf("Fatal failed to retrieve object store admin url : %s \n", err))
	}
	for _, tenant := range tenantList {
		accountsURL := strings.Join([]string{objectStoreURL, "v1/AUTH_", tenant.ID}, "")
		resp, _ := provider.Request("GET", accountsURL, gophercloud.RequestOpts{OkCodes: []int{200}})
		fmt.Println(resp)
	}
	return
}
