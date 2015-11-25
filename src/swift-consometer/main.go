package main

import (
	"encoding/json"
	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

var log = logrus.New()

func checkConfigFile() {
	mandatoryKeys := []string{"credentials.openstack.keystone_uri",
		"credentials.openstack.swift_conso_user",
		"credentials.openstack.swift_conso_password",
		"credentials.openstack.swift_conso_tenant",
		"credentials.rabbit.host",
		"credentials.rabbit.user",
		"credentials.rabbit.password",
		"credentials.rabbit.exchange",
		"credentials.rabbit.routing_key",
		"credentials.rabbit.vhost",
		"credentials.openstack.os_region_name",
		"concurrency",
		"log_level"}

	for _, key := range mandatoryKeys {
		if !viper.IsSet(key) {
			log.Fatal("Incomplete configuration. Missing: ", key)
		}
	}
}

func failOnError(msg string, err error) {
	if err != nil {
		log.Fatal(msg, err)
	}
}

func serviceGet(client *gophercloud.ServiceClient, path string) []byte {
	token := client.TokenID
	URL := strings.Join([]string{client.ServiceURL(), path}, "")
	req, err := http.NewRequest("GET", URL, nil)
	failOnError("Failed creating the request:\n", err)
	req.Header.Set("X-Auth-Token", token)
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	failOnError("Request failed:\n", err)
	if status := resp.StatusCode; status != http.StatusOK {
		log.Fatal("Bad response status when getting ", path, " (expecting 200):\n", status)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	failOnError("Could not read body from request:\n", err)
	return body
}

type servicesCatalog struct {
	Services []struct {
		Description string `json:"description"` //"Nova Compute Service"
		Enabled     bool   `json:"enabled"`     //true
		Id          string `json:"id"`          //"1999c3a858c7408fb586817620695098"
		Links       struct {
			Self string `json:"self"`
		} `json:"links"`
		Name string `json:"name"` //"nova"
		Type string `json:"type"` // compute"
	} `json:"Services"`
	Links struct {
		Self     string  `json:"self"`
		Previous *string `json:"previous"`
		Next     *string `json:"next"`
	} `json:"links"`
}

func getServiceId(client *gophercloud.ServiceClient, serviceType string) string {
	body := serviceGet(client, "services")
	var c servicesCatalog
	err := json.Unmarshal(body, &c)
	failOnError("Failed unmarshalling services catalog:\n", err)
	var result []string
	for _, service := range c.Services {
		if service.Type == serviceType {
			result = append(result, service.Id)
		}
	}
	if len(result) > 1 {
		log.Fatal("Multiple services available with same name:\n", result)
	}
	return result[0]
}

type endpointsCatalog struct {
	Endpoints []struct {
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
		Url        string `json:"url"`
		Region     string `json:"region"`
		Enabled    bool   `json:"enabled"`
		Interface  string `json:"interface"`
		Service_id string `json:"service_id"`
		Id         string `json:"id"`
	} `json:"endpoints"`
	Links struct {
		Self     string  `json:"self"`
		Previous *string `json:"previous"`
		Next     *string `json:"next"`
	} `json:"links"`
}

func getEndpoint(client *gophercloud.ServiceClient, serviceType string, region string, eInterface string) string {
	body := serviceGet(client, "endpoints")
	var c endpointsCatalog
	err := json.Unmarshal(body, &c)
	failOnError("Failed unmarshalling endpoint catalog:\n", err)
	serviceId := getServiceId(client, serviceType)
	var result []string
	for _, endpoint := range c.Endpoints {
		if endpoint.Region == region && endpoint.Service_id == serviceId && endpoint.Interface == eInterface {
			result = append(result, endpoint.Url)
		}
	}
	if len(result) > 1 {
		log.Fatal("Multiple endpoints available:\n", result)
	}
	return result[0]
}

type projectsList struct {
	Projects []struct {
		Domain_id string `json:"domain_id"` //"default",
		Enabled   bool   `json:"enabled"`   //true,
		Id        string `json:"id"`        //"0c4e939acacf4376bdcd1129f1a054ad",
		Name      string `json:"name"`      //"admin",
	} `json:"projects"`
}

func getProjects(client *gophercloud.ServiceClient) projectsList {
	body := serviceGet(client, "projects")
	var c projectsList
	err := json.Unmarshal(body, &c)
	failOnError("Failed unmarshalling projects:\n", err)
	return c
}

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
		resp, err := provider.Request("HEAD", accountUrl, gophercloud.RequestOpts{OkCodes: []int{204, 200, 404}})
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

func countErrors(failed <-chan map[error]string) map[string]int {
	c := make(map[string][]string)
	s := make(map[string]int)
	for failure := range failed {
		for errkey, _ := range failure {
			strkey := strings.SplitN(errkey.Error(), ":", 4)
			key := strkey[len(strkey)-1]
			c[key] = append(c[key], failure[errkey])
		}
	}
	for key, _ := range c {
		s[key] = len(c[key])
	}
	for key, _ := range s {
		log.Error(key, "  #", s[key], "#")
	}
	return s
}

type RabbitCreds struct {
	host        string
	user        string
	password    string
	vhost       string
	exchange    string
	routing_key string
	uri         string
}

type Config struct {
	credentials struct {
		rabbit    RabbitCreds
		openstack struct {
			authOptions    gophercloud.AuthOptions
			os_region_name string
		}
	}
	concurrency int
	log_level   string
}

func readConfig(configPath string, logLevel string) Config {
	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")
	viper.AddConfigPath(configPath)
	err := viper.ReadInConfig()
	failOnError("Error reading config file:\n", err)
	checkConfigFile()

	var config Config
	config.credentials.openstack.os_region_name = viper.GetString("credentials.openstack.os_region_name")

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: viper.GetString("credentials.openstack.keystone_uri"),
		Username:         viper.GetString("credentials.openstack.swift_conso_user"),
		Password:         viper.GetString("credentials.openstack.swift_conso_password"),
		TenantName:       viper.GetString("credentials.openstack.swift_conso_tenant"),
	}
	config.credentials.openstack.authOptions = opts

	rabbit := RabbitCreds{
		host:        viper.GetString("credentials.rabbit.host"),
		user:        viper.GetString("credentials.rabbit.user"),
		password:    viper.GetString("credentials.rabbit.password"),
		vhost:       viper.GetString("credentials.rabbit.vhost"),
		exchange:    viper.GetString("credentials.rabbit.exchange"),
		routing_key: viper.GetString("credentials.rabbit.routing_key"),
	}
	rabbit.uri = strings.Join([]string{"amqp://", rabbit.user, ":", rabbit.password, "@", rabbit.host, "/", rabbit.vhost}, "")
	config.credentials.rabbit = rabbit

	config.concurrency = viper.GetInt("concurrency")

	config.log_level = viper.GetString("log_level")
	if logLevel != "" {
		parsedLogLevel, err := logrus.ParseLevel(logLevel)
		failOnError("Bad log level:\n", err)
		log.Level = parsedLogLevel
	} else {
		parsedLogLevel, err := logrus.ParseLevel(config.log_level)
		failOnError("Bad log level:\n", err)
		log.Level = parsedLogLevel
	}
	log.Debug("Config read:\n", viper.AllSettings())

	return config
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
	start := time.Now()
	for _, project := range projects {
		wg.Add(1)
		sem <- true
		go getAccountInfo(objectStoreURL, project.Id, results, &wg, sem, provider, failedAccounts)
	}
	log.Info("All jobs launched !")
	wg.Wait()
	log.Info("Processed ", len(projects), " tenants in ", time.Since(start))
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
	log.Debug("Created ", len(rbMsg), "B length body:\n", string(rbMsg))

	log.Info("Connecting to:\n", rabbitCreds.uri)
	conn, err := amqp.Dial(rabbitCreds.uri)
	failOnError("Failed to connect to RabbitMQ", err)
	defer conn.Close()
	ch, err := conn.Channel()
	failOnError("Failed to open a channel", err)
	defer ch.Close()

	err = ch.Publish(
		rabbitCreds.exchange,    // exchange
		rabbitCreds.routing_key, // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        []byte(rbMsg),
		})
	failOnError("Failed to publish the message:\n", err)
	log.Info("Message sent!")
	return
}
