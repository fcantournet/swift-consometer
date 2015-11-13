package main

import (
	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/openstack/identity/v2/tenants"
	"github.com/rackspace/gophercloud/pagination"
	"github.com/spf13/viper"
	"net/http"
	"strings"
	"sync"
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
		failOnError("Error processing pager:\n", err)
		return true, nil
	})
	return list
}

/*
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
*/

func worker(id int, jobs <-chan string, results chan<- *http.Response, wg sync.WaitGroup, provider *gophercloud.ProviderClient) {
	for {
		defer wg.Done()
		job := <-jobs
		resp, _ := provider.Request("GET", job, gophercloud.RequestOpts{OkCodes: []int{200}})
		log.Debug("worker", id, ":\n", resp)
		results <- resp
	}
}

func sliceMaker(results <-chan *http.Response) []*http.Response {
	var s []*http.Response
	for range results {
		result := <-results
		s = append(s, result)
	}
	return s
}

func main() {
	ConfigPath := flag.String("config", "./etc/swift", "Path of the configuration file directory.")
	logLevel := flag.Bool("debug", false, "Set log level to debug.")
	workerNumber := flag.Int("n", 3, "Number of goroutines to launch.")
	flag.Parse()
	if *logLevel {
		log.Level = logrus.DebugLevel
	}

	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")                // name of config file (without extension)
	viper.AddConfigPath(*ConfigPath)                 // path to look for the config file in
	err := viper.ReadInConfig()                      // Find and read the config files
	failOnError("Error reading config file:\n", err) // Handle errors reading the config file
	log.Debug("Config used:\n", viper.AllSettings())

	regionName := viper.GetString("os_region_name")
	//	opts, err := buildAuthOptions()
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: viper.GetString("credentials.keystone_uri"),
		Username:         viper.GetString("credentials.swift_conso_user"),
		Password:         viper.GetString("credentials.swift_conso_password"),
		TenantName:       viper.GetString("credentials.swift_conso_tenant"),
	}

	provider, err := openstack.AuthenticatedClient(opts)
	failOnError("Error creating provider:\n", err)

	idClient := openstack.NewIdentityV2(provider)
	tenantList := getTenants(idClient)

	objectStoreURL, err := provider.EndpointLocator(gophercloud.EndpointOpts{Type: "object-store", Region: regionName, Availability: gophercloud.AvailabilityAdmin})
	failOnError("Error retrieving object store admin url:\n", err)
	log.Debug("Object store url:\n", objectStoreURL)

	jobs := make(chan string, len(tenantList))
	results := make(chan *http.Response, len(tenantList))
	var wg sync.WaitGroup
	for w := 1; w <= *workerNumber; w++ {
		go worker(w, jobs, results, wg, provider)
	}
	for _, tenant := range tenantList {
		wg.Add(1)
		jobs <- strings.Join([]string{objectStoreURL, "v1/AUTH_", tenant.ID}, "")
	}
	close(jobs)
	wg.Wait()
	log.Debug("All jobs done")
	close(results)
	respList := sliceMaker(results)
	log.Debug(respList)
	return
}
