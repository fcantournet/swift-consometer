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
		"credentials.openstack.swift_conso_domain",
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

type rabbitCreds struct {
	Host       string
	User       string
	Password   string
	Vhost      string
	Exchange   string
	RoutingKey string
	URI        string
}

type config struct {
	Credentials struct {
		Rabbit    rabbitCreds
		Openstack struct {
			AuthOptions  gophercloud.AuthOptions
			OsRegionName string
		}
	}
	Concurrency int
	LogLevel    string
}

func readConfig(configPath string, logLevel string) config {
	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")
	viper.AddConfigPath(configPath)
	err := viper.ReadInConfig()
	failOnError("Error reading config file:\n", err)
	checkConfigFile()

	var conf config
	conf.Credentials.Openstack.OsRegionName = viper.GetString("credentials.openstack.os_region_name")

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: viper.GetString("credentials.openstack.keystone_uri"),
		Username:         viper.GetString("credentials.openstack.swift_conso_user"),
		Password:         viper.GetString("credentials.openstack.swift_conso_password"),
		TenantName:       viper.GetString("credentials.openstack.swift_conso_tenant"),
		DomainName:       viper.GetString("credentials.openstack.swift_conso_domain"),
	}
	conf.Credentials.Openstack.AuthOptions = opts

	rabbit := rabbitCreds{
		Host:       viper.GetString("credentials.rabbit.host"),
		User:       viper.GetString("credentials.rabbit.user"),
		Password:   viper.GetString("credentials.rabbit.password"),
		Vhost:      viper.GetString("credentials.rabbit.vhost"),
		Exchange:   viper.GetString("credentials.rabbit.exchange"),
		RoutingKey: viper.GetString("credentials.rabbit.routing_key"),
	}
	rabbit.URI = strings.Join([]string{"amqp://", rabbit.User, ":", rabbit.Password, "@", rabbit.Host, "/", rabbit.Vhost}, "")
	conf.Credentials.Rabbit = rabbit

	conf.Concurrency = viper.GetInt("concurrency")

	conf.LogLevel = viper.GetString("log_level")
	if logLevel != "" {
		parsedLogLevel, err := logrus.ParseLevel(logLevel)
		failOnError("Bad log level:\n", err)
		log.Level = parsedLogLevel
	} else {
		parsedLogLevel, err := logrus.ParseLevel(conf.LogLevel)
		failOnError("Bad log level:\n", err)
		log.Level = parsedLogLevel
	}
	log.Debug("Config read:\n", viper.AllSettings())

	return conf
}
