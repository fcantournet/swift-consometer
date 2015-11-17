package main

import (
	"encoding/json"
	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/openstack/identity/v2/tenants"
	"github.com/rackspace/gophercloud/pagination"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"
	"strings"
	"sync"
	"time"
)

var log = logrus.New()

func checkConfig() {
	mandatoryKeys := []string{"credentials.keystone_uri",
		"credentials.swift_conso_user",
		"credentials.swift_conso_password",
		"credentials.swift_conso_tenant",
		"rabbit.host",
		"rabbit.user",
		"rabbit.password",
		"os_region_name"}

	for _, key := range mandatoryKeys {
		if !viper.IsSet(key) {
			log.Fatal("Incomplete Config. Missing: ", key)
		}

	}
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
	Counter_name      string `json:"counter_name"`       //"storage.objects.size",
	Resource_id       string `json:"counter_name"`       //"d5bbc7c06c9e479dbb91912c045cdeab",
	Message_id        string `json:"message_id"`         //"1",
	Timestamp         string `json:"timestamp"`          // "2013-05-13T14:03:01Z",
	Counter_volume    string `json:"counter_volume"`     // "0",
	User_id           string `json:"user_id"`            // null,
	Source            string `json:"source"`             // "openstack",
	Counter_unit      string `json:"counter_unit"`       // "B",
	Project_id        string `json:"project_id"`         // "d5bbc7c06c9e479dbb91912c045cdeab",
	Counter_type      string `json:"counter_type"`       // "gauge",
	Resource_metadata string `json:"ressource_metadata"` // null
}

type rabbitPayload struct {
	Args struct {
		Data []accountInfo `json:"data"`
	} `json:"args"`
}

func getAccountInfo(objectStoreURL, tenantID string, results chan<- accountInfo, wg *sync.WaitGroup, provider *gophercloud.ProviderClient) {
	defer wg.Done()
	accountUrl := strings.Join([]string{objectStoreURL, "v1/AUTH_", tenantID}, "")
	var max_retries int = 1
	for i := 0; i <= max_retries; i++ {
		resp, err := provider.Request("GET", accountUrl, gophercloud.RequestOpts{OkCodes: []int{200}})
		if err != nil {
			if i == max_retries {
				log.Error("Failed to fetch account info : ", err)
				continue
			}
			log.Warn("Failed to fetch account info : ", err, "  Retrying(", i, ")")
			time.Sleep(100 * time.Millisecond)
			continue
		}
		ai := accountInfo{
			Counter_name:      "storage.objects.size",
			Resource_id:       tenantID,
			Message_id:        "1",
			Timestamp:         resp.Header.Get("x-timestamp"),
			Counter_volume:    resp.Header.Get("x-account-bytes-used"),
			User_id:           "",
			Source:            "openstack",
			Counter_unit:      "B",
			Project_id:        tenantID,
			Counter_type:      "gauge",
			Resource_metadata: "",
		}
		log.Debug("Fetched account: ", accountUrl)
		results <- ai
		return
	}
	// TODO: Add potential error management when account couldn't be queried
}

func aggregateResponses(results <-chan accountInfo) []accountInfo {
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
	flag.Parse()
	if *logLevel {
		log.Level = logrus.DebugLevel
	}

	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")
	viper.AddConfigPath(*ConfigPath)
	err := viper.ReadInConfig()
	failOnError("Error reading config file:\n", err)
	checkConfig()
	log.Debug("Config used:\n", viper.AllSettings())
	regionName := viper.GetString("os_region_name")
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

	// Buffered chan can take all the answers
	results := make(chan accountInfo, len(tenantList))
	var wg sync.WaitGroup
	start := time.Now()
	for _, tenant := range tenantList {
		wg.Add(1)
		go getAccountInfo(objectStoreURL, tenant.ID, results, &wg, provider)
	}
	log.Debug("All jobs launched !")
	wg.Wait()
	log.Debug("Processed ", len(tenantList), " tenants in ", time.Since(start))
	log.Debug("All jobs done")
	close(results)
	respList := aggregateResponses(results)
	rbMsg := rabbitPayload{}
	rbMsg.Args.Data = respList
	output, _ := json.Marshal(rbMsg)
	log.Debug(string(output))
	return

	rabbitCreds := map[string]string{
		"host":     viper.GetString("rabbit.host"),
		"user":     viper.GetString("rabbit.user"),
		"password": viper.GetString("rabbit.password"),
	}

	rabbitURI := strings.Join([]string{"amqp://", rabbitCreds["user"], ":", rabbitCreds["password"], "@", rabbitCreds["host"]}, "")
	log.Debug("Rabbit used:\n", rabbitURI)

	conn, err := amqp.Dial(rabbitURI)
	failOnError("Failed to connect to RabbitMQ", err)
	defer conn.Close()
	ch, err := conn.Channel()
	failOnError("Failed to open a channel", err)
	defer ch.Close()

	//TODO Get the right options
	err = ch.Publish(
		"",    // exchange
		"bob", // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(output),
		})
	failOnError("Failed to publish the message:\n", err)
	log.Debug("Sent to RabbitMq:\n", output)
	return
}
