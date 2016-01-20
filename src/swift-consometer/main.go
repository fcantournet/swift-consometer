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

func failOnError(msg string, err error) {
	if err != nil {
		log.Fatal(msg, err)
	}
}

func getAccountInfo(objectStoreURL, projectID string, results chan<- accountInfo, wg *sync.WaitGroup, sem <-chan bool, provider *gophercloud.ProviderClient, failedAccounts chan<- map[error]string) {
	defer wg.Done()
	defer func() { <-sem }()
	accountURL := strings.Join([]string{objectStoreURL, "/v1/AUTH_", projectID}, "")
	var maxRetries = 2
	for i := 0; i <= maxRetries; i++ {
		resp, err := provider.Request("HEAD", accountURL, gophercloud.RequestOpts{OkCodes: []int{204, 200}})
		if err != nil {
			if i < maxRetries {
				log.Debug(err, " Retry ", i+1)
				time.Sleep(100 * time.Millisecond)
				continue
			} else {
				log.Error(err)
				failedAccounts <- map[error]string{err: projectID}
				return
			}
		}
		log.Debug("Fetching account: ", accountURL)
		ai := accountInfo{
			CounterName:      "storage.objects.size",
			ResourceID:       projectID,
			MessageID:        uuid.New(),
			Timestamp:        time.Now().Format(time.RFC3339),
			CounterVolume:    resp.Header.Get("x-account-bytes-used"),
			UserID:           nil,
			Source:           "openstack",
			CounterUnit:      "B",
			ProjectID:        projectID,
			CounterType:      "gauge",
			ResourceMetadata: nil,
		}
		results <- ai
		return
	}
}

func aggregateResponses(results <-chan accountInfo, chunkSize int) [][]accountInfo {
	var r [][]accountInfo
	for {
		if len(results) <= chunkSize {
			var s []accountInfo
			for result := range results {
				s = append(s, result)
			}
			r = append(r, s)
			return r
		}
		var s []accountInfo
		for i := 0; i < chunkSize; i++ {
			s = append(s, <-results)
		}
		r = append(r, s)
	}
}

func rabbitSend(rabbit rabbitCreds, rbMsgs [][]byte) {
	log.Debug("Connecting to: ", rabbit.URI)
	conn, err := amqp.Dial(rabbit.URI)
	failOnError("Failed to connect to RabbitMQ", err)
	defer conn.Close()
	ch, err := conn.Channel()
	failOnError("Failed to open channel", err)
	defer ch.Close()

	log.Debug("Checking existence or declaring exchange: ", rabbit.Exchange)
	if err = ch.ExchangeDeclare(
		rabbit.Exchange, // name of the exchange
		"topic",         // type
		false,           // durable
		false,           // delete when complete
		false,           // internal
		false,           // noWait
		nil,             // arguments
	); err != nil {
		log.Fatal("Failed declaring exchange: ", err)
	}

	log.Debug("Checking existence or declaring queue: ", rabbit.Queue)
	_, err = ch.QueueDeclare(
		rabbit.Queue, // name of the queue
		true,         // durable
		false,        // delete when usused
		false,        // exclusive
		false,        // noWait
		nil,          // arguments
	)
	if err != nil {
		log.Fatal("Failed declaring queue: ", err)
	}

	log.Debug("Binding queue to exchange")
	if err = ch.QueueBind(
		rabbit.Queue,      // name of the queue
		rabbit.RoutingKey, // bindingKey
		rabbit.Exchange,   // sourceExchange
		false,             // noWait
		nil,               // arguments
	); err != nil {
		log.Fatal("Failed binding queue: ", err)
	}

	nbSent := 1
	for _, rbMsg := range rbMsgs {
		log.Debug("Sending ", nbSent, " out of ", len(rbMsgs), " message with ", len(rbMsg), "B length body")
		log.Debug(string(rbMsg))
		err = ch.Publish(
			rabbit.Exchange,   // exchange
			rabbit.RoutingKey, // routing key
			false,             // mandatory
			false,             // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte(rbMsg),
			})
		failOnError("Failed to publish message: ", err)
		nbSent++
	}
	return
}

func main() {
	log.Info("Starting...")
	configPath := flag.String("config", "/etc/swift_consometer/", "Path of the configuration file directory.")
	logLevel := flag.String("l", "", "Set log level info|debug|warn|error|panic. Default is info.")
	flag.Parse()

	conf := readConfig(*configPath, *logLevel)
	regions := conf.Regions
	opts := conf.Credentials.Openstack.AuthOptions
	rabbitCreds := conf.Credentials.Rabbit
	concurrency := conf.Concurrency

	provider, err := openstack.AuthenticatedClient(opts)
	failOnError("Failed creating provider: ", err)

	idClient := openstack.NewIdentityV3(provider)
	pList := getProjects(idClient)
	projects := pList.Projects
	log.Info(len(projects), " projects retrieved")
	log.Debug(projects)

	for _, region := range regions {
		log.Info("Working on region: ", region)
		objectStoreURL := getEndpoint(idClient, "object-store", region, "admin")
		log.Debug("Object store url: ", objectStoreURL)

		// Buffered chan can take all the answers
		results := make(chan accountInfo, len(projects))
		failedAccounts := make(chan map[error]string, len(projects))
		sem := make(chan bool, concurrency)

		var wg sync.WaitGroup
		log.Debug("Launching jobs")
		start := time.Now()
		for _, project := range projects {
			wg.Add(1)
			sem <- true
			go getAccountInfo(objectStoreURL, project.ID, results, &wg, sem, provider, failedAccounts)
		}
		wg.Wait()
		close(results)
		close(failedAccounts)
		close(sem)

		//failedAccounts channel may be more useful for error management in the future
		if len(failedAccounts) > 0 {
			log.Error("Number of accounts failed: ", len(failedAccounts))
		}
		log.Info(len(results), " swift accounts fetched out of ", len(projects), " projects in ", time.Since(start))

		respList := aggregateResponses(results, 200) //Chunks of 300 accounts, roughly 100KB per message
		nmbMsgs := 1
		var rbMsgs [][]byte
		for _, chunk := range respList {
			output := rabbitPayload{}
			output.Args.Data = chunk
			rbMsg, _ := json.Marshal(output)
			nmbMsgs++
			rbMsgs = append(rbMsgs, rbMsg)
		}
		log.Info("Sending results to queue")
		rabbitSend(rabbitCreds, rbMsgs)
		log.Info("Job done")
	}

	return
}
