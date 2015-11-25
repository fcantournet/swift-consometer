package main

import (
	"encoding/json"
	"flag"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/streadway/amqp"
	"strings"
	"sync"
	"time"
)

type accountInfo struct {
	Counter_name      string  `json:"counter_name"`       //"storage.objects.size",
	Resource_id       string  `json:"resource_id"`        //"d5bbc7c06c9e479dbb91912c045cdeab",
	Message_id        string  `json:"message_id"`         //"1",
	Timestamp         string  `json:"timestamp"`          // "2013-05-13T14:03:01Z",
	Counter_volume    string  `json:"counter_volume"`     // "0",
	User_id           *string `json:"user_id"`            // null,
	Source            string  `json:"source"`             // "openstack",
	Counter_unit      string  `json:"counter_unit"`       // "B",
	Project_id        string  `json:"project_id"`         // "d5bbc7c06c9e479dbb91912c045cdeab",
	Counter_type      string  `json:"counter_type"`       // "gauge",
	Resource_metadata *string `json:"ressource_metadata"` // null
}

type rabbitPayload struct {
	Args struct {
		Data []accountInfo `json:"data"`
	} `json:"args"`
}

func getAccountInfo(objectStoreURL, tenantID string, results chan<- accountInfo, wg *sync.WaitGroup, sem <-chan bool, provider *gophercloud.ProviderClient, failedAccounts chan<- map[error]string) {
	defer wg.Done()
	defer func() { <-sem }()
	accountUrl := strings.Join([]string{objectStoreURL, "/v1/AUTH_", tenantID}, "")
	var max_retries int = 2
	for i := 0; i <= max_retries; i++ {
		resp, err := provider.Request("HEAD", accountUrl, gophercloud.RequestOpts{OkCodes: []int{204, 200}})
		if err != nil {
			if i < max_retries {
				log.Warn(err, " Retry ", i+1)
				time.Sleep(100 * time.Millisecond)
				continue
			} else {
				log.Error(err)
				failedAccounts <- map[error]string{err: tenantID}
				return
			}
		}
		ai := accountInfo{
			Counter_name:      "storage.objects.size",
			Resource_id:       tenantID,
			Message_id:        "1",
			Timestamp:         resp.Header.Get("x-timestamp"),
			Counter_volume:    resp.Header.Get("x-account-bytes-used"),
			User_id:           nil,
			Source:            "openstack",
			Counter_unit:      "B",
			Project_id:        tenantID,
			Counter_type:      "gauge",
			Resource_metadata: nil,
		}
		log.Debug("Fetched account: ", accountUrl)
		results <- ai
		return
	}
	// TODO: Add potential error management when account couldn't be queried
}

func aggregateResponses(results <-chan accountInfo) []accountInfo {
	var s []accountInfo
	for result := range results {
		s = append(s, result)
	}
	return s
}
func rabbitSend(rabbit RabbitCreds, rbMsg []byte) {
	log.Info("Connecting to:\n", rabbit.uri)
	conn, err := amqp.Dial(rabbit.uri)
	failOnError("Failed to connect to RabbitMQ", err)
	defer conn.Close()
	ch, err := conn.Channel()
	failOnError("Failed to open a channel", err)
	defer ch.Close()

	err = ch.Publish(
		rabbit.exchange,    // exchange
		rabbit.routing_key, // routing key
		false,              // mandatory
		false,              // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        []byte(rbMsg),
		})
	failOnError("Failed to publish the message:\n", err)
	log.Info("Message sent!")
	return
}

func main() {
	configPath := flag.String("config", "./etc/swift", "Path of the configuration file directory.")
	logLevel := flag.String("l", "", "Set log level {info, debug, warn, error, panic}. Default is info.")
	flag.Parse()

	config := readConfig(*configPath, *logLevel)
	regionName := config.credentials.openstack.os_region_name
	opts := config.credentials.openstack.authOptions
	rabbitCreds := config.credentials.rabbit
	concurrency := config.concurrency

	provider, err := openstack.AuthenticatedClient(opts)
	failOnError("Error creating provider:\n", err)

	idClient := openstack.NewIdentityV3(provider)
	pList := getProjects(idClient)
	projects := pList.Projects
	log.Info("Number of projects fetched: ", len(projects))
	log.Debug(projects)

	objectStoreURL := getEndpoint(idClient, "object-store", regionName, "admin")
	log.Debug("Object store url:\n", objectStoreURL)

	// Buffered chan can take all the answers
	results := make(chan accountInfo, len(projects))
	failedAccounts := make(chan map[error]string, len(projects))
	sem := make(chan bool, concurrency)

	var wg sync.WaitGroup
	log.Info("Launching jobs")
	start := time.Now()
	for _, project := range projects {
		wg.Add(1)
		sem <- true
		go getAccountInfo(objectStoreURL, project.Id, results, &wg, sem, provider, failedAccounts)
	}
	wg.Wait()
	log.Info("Processed ", len(results), " tenants in ", time.Since(start))
	close(results)
	close(failedAccounts)
	close(sem)
	if len(failedAccounts) > 0 {
		log.Error("Number of accounts failed: ", len(failedAccounts))
		countErrors(failedAccounts)
	}

	//FIXME: return is for test purposes
	return

	respList := aggregateResponses(results)

	output := rabbitPayload{}
	output.Args.Data = respList
	rbMsg, _ := json.Marshal(output)
	log.Info("Created ", len(rbMsg), "B length body")
	log.Debug(string(rbMsg))
	rabbitSend(rabbitCreds, rbMsg)
	return
}
