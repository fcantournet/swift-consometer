package main

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rackspace/gophercloud"
	"io/ioutil"
	"net/http"
	"strings"
)

func serviceGet(client *gophercloud.ServiceClient, path string) ([]byte, error) {
	token := client.TokenID
	URL := strings.Join([]string{client.ServiceURL(), path}, "")
	req, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		return []byte{}, errors.Wrap(err, "Failed creating request")
	}
	req.Header.Set("X-Auth-Token", token)
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return []byte{}, errors.Wrap(err, "Request failed")
	}
	if status := resp.StatusCode; status != http.StatusOK {
		return []byte{}, errors.New(fmt.Sprintf("Bad response status when getting %s (expecting 200 OK): %s", path, resp.Status))
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, errors.Wrap(err, "Could not read body from request")
	}
	return body, nil
}

type servicesCatalog struct {
	Services []struct {
		Description string `json:"description"` //"Nova Compute Service"
		Enabled     bool   `json:"enabled"`     //true
		ID          string `json:"id"`          //"1999c3a858c7408fb586817620695098"
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

func getServiceID(client *gophercloud.ServiceClient, serviceType string) (string, error) {
	body, err := serviceGet(client, "services")
	if err != nil {
		return "", errors.Wrap(err, "Could not get sercices")
	}
	var c servicesCatalog
	if err := json.Unmarshal(body, &c); err != nil {
		return "", errors.Wrap(err, "Failed unmarshalling services catalog")
	}
	var result []string
	for _, service := range c.Services {
		if service.Type == serviceType {
			result = append(result, service.ID)
		}
	}
	if len(result) > 1 {
		return "", errors.New(fmt.Sprintf(" %v\nMultiple services available with same name", result))
	}
	return result[0], nil
}

type endpointsCatalog struct {
	Endpoints []struct {
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
		URL       string `json:"url"`
		Region    string `json:"region"`
		Enabled   bool   `json:"enabled"`
		Interface string `json:"interface"`
		ServiceID string `json:"service_id"`
		ID        string `json:"id"`
	} `json:"endpoints"`
	Links struct {
		Self     string  `json:"self"`
		Previous *string `json:"previous"`
		Next     *string `json:"next"`
	} `json:"links"`
}

func getEndpoint(client *gophercloud.ServiceClient, serviceType string, region string, eInterface string) (string, error) {
	body, err := serviceGet(client, "endpoints")
	if err != nil {
		return "", errors.Wrap(err, "Could not get endpoints")
	}
	var c endpointsCatalog
	if err := json.Unmarshal(body, &c); err != nil {
		return "", errors.Wrap(err, "Failed unmarshalling endpoint catalog")
	}
	serviceID, err := getServiceID(client, serviceType)
	if err != nil {
		return "", errors.Wrap(err, "Could not get serviceID")
	}
	var result []string
	for _, endpoint := range c.Endpoints {
		if endpoint.Region == region && endpoint.ServiceID == serviceID && endpoint.Interface == eInterface {
			result = append(result, endpoint.URL)
		}
	}
	if len(result) > 1 {
		return "", errors.New(fmt.Sprintf("Multiple endpoints available: %v", result))
	}
	if len(result) < 1 {
		return "", errors.New(fmt.Sprintf("No endpoint for service %s in region %s", serviceType, region))
	}
	return result[0], nil
}

type projectsList struct {
	Projects []struct {
		DomainID string `json:"domain_id"` //"default",
		Enabled  bool   `json:"enabled"`   //true,
		ID       string `json:"id"`        //"0c4e939acacf4376bdcd1129f1a054ad",
		Name     string `json:"name"`      //"admin",
	} `json:"projects"`
}

func getProjects(client *gophercloud.ServiceClient) (projectsList, error) {
	var c projectsList
	body, err := serviceGet(client, "projects")
	if err != nil {
		return c, errors.Wrap(err, "Could not get projects")
	}
	if err := json.Unmarshal(body, &c); err != nil {
		return c, errors.Wrap(err, "Failed unmarshalling projects")
	}
	return c, nil
}
