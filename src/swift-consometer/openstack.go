package main

import (
	"encoding/json"
	"github.com/rackspace/gophercloud"
	"io/ioutil"
	"net/http"
	"strings"
)

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
