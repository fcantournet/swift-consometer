package main

import (
	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/openstack/identity/v2/tenants"
	"github.com/rackspace/gophercloud/pagination"
	"github.com/spf13/viper"
	"strings"
)

var log = logrus.New()

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
		log.Fatal("Unable to decode into struct: ", err)
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

func failOnError(msg string, err error) {
	if err != nil {
		log.Fatal(msg, err)
	}
}

func getTenants(client *gophercloud.ServiceClient) []tenants.Tenant {
	var list []tenants.Tenant
	pager := tenants.List(client, nil)
	pager.EachPage(func(page pagination.Page) (bool, error) {
		tenantList, err := tenants.ExtractTenants(page)
		list = append(tenantList)
		failOnError("Error processing pager: ", err)
		return true, nil
	})
	return list
}

func main() {
	ConfigPath := flag.String("config", "./etc/swift", "Path of the configuration file directory.")
	logLevel := flag.Bool("debug", false, "Set log level to debug.")
	flag.Parse()
	if *logLevel {
		log.Level = logrus.DebugLevel
	}

	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")               // name of config file (without extension)
	viper.AddConfigPath(*ConfigPath)                // path to look for the config file in
	err := viper.ReadInConfig()                     // Find and read the config files
	failOnError("Error reading config file: ", err) // Handle errors reading the config file
	log.Debug("Config used: \n", viper.AllSettings())

	regionName := viper.GetString("os_region_name")
	//	opts, err := buildAuthOptions()
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: viper.GetString("credentials.keystone_uri"),
		Username:         viper.GetString("credentials.swift_conso_user"),
		Password:         viper.GetString("credentials.swift_conso_password"),
		TenantName:       viper.GetString("credentials.swift_conso_tenant"),
	}

	provider, err := openstack.AuthenticatedClient(opts)
	failOnError("Error creating provider: ", err)

	idClient := openstack.NewIdentityV2(provider)
	tenantList := getTenants(idClient)

	objectStoreURL, err := provider.EndpointLocator(gophercloud.EndpointOpts{Type: "object-store", Region: regionName, Availability: gophercloud.AvailabilityAdmin})
	failOnError("Error retrieving object store admin url: ", err)
	log.Debug("object store url: ", objectStoreURL)

	for _, tenant := range tenantList {
		accountsURL := strings.Join([]string{objectStoreURL, "v1/AUTH_", tenant.ID}, "")
		resp, _ := provider.Request("GET", accountsURL, gophercloud.RequestOpts{OkCodes: []int{200}})
		log.Debug(resp)
	}

	return
}
