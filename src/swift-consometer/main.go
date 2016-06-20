package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/streadway/amqp"
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
	Region           string  `json:"region"`             // "int5"
}

type failedAccountQuery struct {
	ProjectID string
	Error     error
	Retry     int
}

type rabbitPayload struct {
	Args struct {
		Data []accountInfo `json:"data"`
	} `json:"args"`
}

func getAccountInfo(region, objectStoreURL, projectID string, results chan<- accountInfo, wga *sync.WaitGroup, provider *gophercloud.ProviderClient, failedAccountQueries chan<- failedAccountQuery) {
	defer wga.Done()
	accountURL := strings.Join([]string{objectStoreURL, "/v1/AUTH_", projectID}, "")
	var maxRetries = 2
	for i := 0; i <= maxRetries; i++ {
		resp, err := provider.Request("HEAD", accountURL, gophercloud.RequestOpts{OkCodes: []int{204, 200}})
		if err != nil {
			if i < maxRetries {
				log.Debug(err, " Retry ", i+1)
				<-time.Tick(1000 * time.Millisecond)
				continue
			} else {
				log.Error(err)
				failedAccountQueries <- failedAccountQuery{
					ProjectID: projectID,
					Error:     err,
					Retry:     i}
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
			Region:           region,
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

func rabbitSend(rabbit rabbitCreds, rbMsgs [][]byte) error {
	log.Debug("Connecting to: ", rabbit.URI)
	conn, err := amqp.Dial(rabbit.URI)
	if err != nil {
		return errors.Wrap(err, "Failed to connect to RabbitMQ")
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return errors.Wrap(err, "Failed to open channel")
	}
	defer ch.Close()

	log.Debug("Checking existence or declaring exchange: ", rabbit.Exchange)
	if err := ch.ExchangeDeclare(
		rabbit.Exchange, // name of the exchange
		"topic",         // type
		false,           // durable
		false,           // delete when complete
		false,           // internal
		false,           // noWait
		nil,             // arguments
	); err != nil {
		return errors.Wrap(err, "Failed declaring exchange")
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
		return errors.Wrap(err, "Failed declaring queue")
	}

	log.Debug("Binding queue to exchange")
	if err := ch.QueueBind(
		rabbit.Queue,      // name of the queue
		rabbit.RoutingKey, // bindingKey
		rabbit.Exchange,   // sourceExchange
		false,             // noWait
		nil,               // arguments
	); err != nil {
		return errors.Wrap(err, "Failed binding queue")
	}

	nbSent := 1
	for _, rbMsg := range rbMsgs {
		log.Debug("Sending ", nbSent, " out of ", len(rbMsgs), " message with ", len(rbMsg), "B length body")
		log.Debug(string(rbMsg))
		if err := ch.Publish(
			rabbit.Exchange,   // exchange
			rabbit.RoutingKey, // routing key
			false,             // mandatory
			false,             // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte(rbMsg),
			}); err != nil {
			return errors.Wrap(err, "Failed to publish message")
		}
		nbSent++
	}
	return nil
}

func main() {
	log.Info("Starting...")
	configPath := flag.String("config", "/etc/swift_consometer/", "Path of the configuration file directory.")
	logLevel := flag.String("l", "", "Set log level info|debug|warn|error|panic. Default is info.")
	flag.Parse()

	conf, err := readConfig(*configPath, *logLevel)
	if err != nil {
		log.Fatal("Failed reading configuration: ", err)
	}

	regions := conf.Regions
	opts := conf.Credentials.Openstack.AuthOptions
	rabbitCreds := conf.Credentials.Rabbit
	ticker := conf.Ticker

	go func() {
		time.Sleep(1800 * time.Second)
		log.Fatal("Timeout")
	}()

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		log.Fatal("Failed creating provider: ", err)
	}

	idClient := openstack.NewIdentityV3(provider)
	pList, err := getProjects(idClient)
	if err != nil {
		log.Fatal("Could not get projects: ", err)
	}

	projects := pList.Projects
	log.Info(len(projects), " projects retrieved")
	log.Debug(projects)

	var wg sync.WaitGroup
	wg.Add(len(regions))
	for _, region := range regions {
		go func(region string) {
			defer wg.Done()
			log.Info(fmt.Sprintf("[%s] Starting", region))
			objectStoreURL, err := getEndpoint(idClient, "object-store", region, "admin")
			if err != nil {
				log.Fatal("Could not get object-store url: ", err)
			}
			log.Debug(fmt.Sprintf("[%s] Object store url: %s", region, objectStoreURL))

			// Buffered chan can take all the answers
			results := make(chan accountInfo, len(projects))
			failedAccountQueries := make(chan failedAccountQuery, len(projects))

			log.Debug(fmt.Sprintf("[%s] Launching jobs", region))
			start := time.Now()

			var wga sync.WaitGroup
			wga.Add(len(projects))
			for _, project := range projects {
				if ticker > 0 {
					<-time.Tick(time.Duration(ticker) * time.Millisecond)
				}
				go getAccountInfo(region, objectStoreURL, project.ID, results, &wga, provider, failedAccountQueries)
			}

			wga.Wait()
			close(results)
			close(failedAccountQueries)

			//failedAccountQueries channel may be more useful for error management in the future
			if len(failedAccountQueries) > 0 {
				log.Error(fmt.Sprintf("[%s] Number of accounts failed: %d", region, len(failedAccountQueries)))
			}
			log.Info(fmt.Sprintf("[%s] %d Swift accounts fetched out of %d projects in %v", region, len(results), len(projects), time.Since(start)))

			respList := aggregateResponses(results, 200) //Chunks of 200 accounts, roughly 100KB per message
			var rbMsgs [][]byte
			for _, chunk := range respList {
				output := rabbitPayload{}
				output.Args.Data = chunk
				rbMsg, _ := json.Marshal(output)
				rbMsgs = append(rbMsgs, rbMsg)
			}
			log.Info(fmt.Sprintf("[%s] Sending results to queue", region))
			if err := rabbitSend(rabbitCreds, rbMsgs); err != nil {
				log.Fatal("Failed sending messages to rabbit", err)
			}
			log.Info(fmt.Sprintf("[%s] Done", region))
		}(region)
	}
	wg.Wait()
	log.Info("Done")
	return
}
