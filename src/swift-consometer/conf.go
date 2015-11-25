package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/rackspace/gophercloud"
	"github.com/spf13/viper"
	"strings"
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
