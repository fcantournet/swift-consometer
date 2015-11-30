package main

import (
	"encoding/json"
	"flag"
	"github.com/pborman/uuid"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/streadway/amqp"
	"strings"
	"sync"
	"time"
)

type accountInfo struct {
	CounterName      string  `json:"counter_name"`       //"storage.objects.size",
	ResourceID       string  `json:"resource_id"`        //"d5bbc7c06c9e479dbb91912c045cdeab",
	MessageID        string  `json:"message_id"`         //"1",
	Timestamp        string  `json:"timestamp"`          // "2013-05-13T14:03:01Z",
	CounterVolume    string  `json:"counter_volume"`     // "0",
	UserID           *string `json:"user_id"`            // null,
	Source           string  `json:"source"`             // "openstack",
	CounterUnit      string  `json:"counter_unit"`       // "B",
	ProjectID        string  `json:"project_id"`         // "d5bbc7c06c9e479dbb91912c045cdeab",
	CounterType      string  `json:"counter_type"`       // "gauge",
	ResourceMetadata *string `json:"ressource_metadata"` // null
}

type rabbitPayload struct {
	Args struct {
		Data []accountInfo `json:"data"`
	} `json:"args"`
}

func getAccountInfo(objectStoreURL, tenantID string, results chan<- accountInfo, wg *sync.WaitGroup, sem <-chan bool, provider *gophercloud.ProviderClient, failedAccounts chan<- map[error]string) {
	defer wg.Done()
	defer func() { <-sem }()
	accountURL := strings.Join([]string{objectStoreURL, "/v1/AUTH_", tenantID}, "")
	var maxRetries = 2
	for i := 0; i <= maxRetries; i++ {
		resp, err := provider.Request("HEAD", accountURL, gophercloud.RequestOpts{OkCodes: []int{204, 200}})
		if err != nil {
			if i < maxRetries {
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
			CounterName:      "storage.objects.size",
			ResourceID:       tenantID,
			MessageID:        uuid.New(),
			Timestamp:        time.Now().Format(time.RFC3339),
			CounterVolume:    resp.Header.Get("x-account-bytes-used"),
			UserID:           nil,
			Source:           "openstack",
			CounterUnit:      "B",
			ProjectID:        tenantID,
			CounterType:      "gauge",
			ResourceMetadata: nil,
		}
		log.Debug("Fetched account: ", accountURL)
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
func rabbitSend(rabbit rabbitCreds, rbMsg []byte) {
	log.Info("Connecting to:\n", rabbit.URI)
	conn, err := amqp.Dial(rabbit.URI)
	failOnError("Failed to connect to RabbitMQ", err)
	defer conn.Close()
	ch, err := conn.Channel()
	failOnError("Failed to open a channel", err)
	defer ch.Close()

	err = ch.Publish(
		rabbit.Exchange,   // exchange
		rabbit.RoutingKey, // routing key
		false,             // mandatory
		false,             // immediate
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

	conf := readConfig(*configPath, *logLevel)
	regionName := conf.Credentials.Openstack.OsRegionName
	opts := conf.Credentials.Openstack.AuthOptions
	rabbitCreds := conf.Credentials.Rabbit
	concurrency := conf.Concurrency

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
		go getAccountInfo(objectStoreURL, project.ID, results, &wg, sem, provider, failedAccounts)
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

	//FIXME: return for debug
	//	return

	respList := aggregateResponses(results)

	output := rabbitPayload{}
	output.Args.Data = respList
	rbMsg, _ := json.Marshal(output)
	log.Info("Created ", len(rbMsg), "B length body")
	log.Debug(string(rbMsg))
	rabbitSend(rabbitCreds, rbMsg)
	return
}
