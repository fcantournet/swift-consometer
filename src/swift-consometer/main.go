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
	"sync"
)

var log = logrus.New()

func buildAuthOptions() (gophercloud.AuthOptions, error) {

	mandatoryKeys := []string{"credentials.keystone_uri",
		"credentials.swift_conso_user",
		"credentials.swift_conso_password",
		"credentials.swift_conso_tenant",
		"os_region_name"}

	for _, key := range mandatoryKeys {
		if !viper.IsSet(key) {
			log.Fatal("Incomplete Config. Missing : ", key)
		}

	}
	ao := gophercloud.AuthOptions{
		IdentityEndpoint: viper.GetString("credentials.keystone_uri"),
		Username:         viper.GetString("credentials.swift_conso_user"),
		Password:         viper.GetString("credentials.swift_conso_password"),
		TenantName:       viper.GetString("credentials.swift_conso_tenant"),
	}
	return ao, nil
}

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
		failOnError("Error processing pager:\n", err)
		return true, nil
	})
	return list
}

type accountInfo struct {
	counter_name      string //"storage.objects.size",
	resource_id       string //"d5bbc7c06c9e479dbb91912c045cdeab",
	message_id        string //"1",
	timestamp         string // "2013-05-13T14:03:01Z",
	counter_volume    string // "0",
	user_id           string // null,
	source            string // "openstack",
	counter_unit      string // "B",
	project_id        string // "d5bbc7c06c9e479dbb91912c045cdeab",
	counter_type      string // "gauge",
	resource_metadata string // null
}

func getAccountInfo(objectStoreURL, tenantID string, results chan<- accountInfo, wg *sync.WaitGroup, provider *gophercloud.ProviderClient) {
	defer wg.Done()
	accountUrl := strings.Join([]string{objectStoreURL, "v1/AUTH_", tenantID}, "")
	for i := 0; i < 1; i++ {
		resp, err := provider.Request("GET", accountUrl, gophercloud.RequestOpts{OkCodes: []int{200}})
		if err != nil {
			log.Debug("Failed to fetch account info : ", err, "  Retrying(", i, ")")
			continue
		}
		ai := accountInfo{
			counter_name:      "storage.objects.size",
			resource_id:       tenantID,
			message_id:        "1",
			timestamp:         resp.Header.Get("x-timestamp"),
			counter_volume:    resp.Header.Get("x-account-bytes-used"),
			user_id:           "",
			source:            "openstack",
			counter_unit:      "B",
			project_id:        tenantID,
			counter_type:      "gauge",
			resource_metadata: "",
		}
		log.Debug("Fetched account: ", accountUrl)
		results <- ai
		return
	}
	// TODO: Add potential error management when account couldn't be queried
}

func sliceMaker(results <-chan accountInfo) []accountInfo {
	var s []accountInfo
	for range results {
		result := <-results
		s = append(s, result)
	}
	return s
}

func main() {
	ConfigPath := flag.String("config", "./etc/swift", "Path of the configuration file directory.")
	logLevel := flag.Bool("debug", false, "Set log level to debug.")
	//workerNumber := flag.Int("n", 3, "Number of goroutines to launch.")
	flag.Parse()
	if *logLevel {
		log.Level = logrus.DebugLevel
	}

	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")
	viper.AddConfigPath(*ConfigPath)
	err := viper.ReadInConfig()
	failOnError("Error reading config file:\n", err)
	log.Debug("Config used:\n", viper.AllSettings())

	if !viper.IsSet("os_region_name") {
		log.Fatal("Missing Region in config !")
	}
	regionName := viper.GetString("os_region_name")

	opts, err := buildAuthOptions()

	provider, err := openstack.AuthenticatedClient(opts)
	failOnError("Error creating provider:\n", err)

	idClient := openstack.NewIdentityV2(provider)
	tenantList := getTenants(idClient)

	objectStoreURL, err := provider.EndpointLocator(gophercloud.EndpointOpts{Type: "object-store", Region: regionName, Availability: gophercloud.AvailabilityAdmin})
	failOnError("Error retrieving object store admin url:\n", err)
	log.Debug("Object store url:\n", objectStoreURL)

	// Buffered chan can take all the answers
	results := make(chan accountInfo, len(tenantList))
	var wg sync.WaitGroup
	for _, tenant := range tenantList {
		wg.Add(1)
		go getAccountInfo(objectStoreURL, tenant.ID, results, &wg, provider)
	}
	log.Debug("All jobs launched !")
	wg.Wait()
	log.Debug("All jobs done")
	close(results)
	respList := sliceMaker(results)
	log.Debug(respList)
	return
}
