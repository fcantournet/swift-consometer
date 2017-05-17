package main

import (
	"fmt"
	"strings"

	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gophercloud/gophercloud"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var log = logrus.New()

func checkConfigFile() error {
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
		"credentials.rabbit.queue",
		"timeout",
		"region",
		"workers",
		"log_level"}

	for _, key := range mandatoryKeys {
		if !viper.IsSet(key) {
			return fmt.Errorf("Incomplete configuration. Missing key %s", key)
		}
	}
	return nil
}

type rabbitCreds struct {
	Host       string
	User       string
	Password   string
	Vhost      string
	Exchange   string
	RoutingKey string
	URI        string
	Queue      string
}

type config struct {
	Credentials struct {
		Rabbit    rabbitCreds
		Openstack struct {
			AuthOptions gophercloud.AuthOptions
		}
	}
	Graphite struct {
		Port     int
		Hostname string
		Prefix   string
	}
	Region   string
	Timeout  time.Duration
	Workers  int
	LogLevel string
}

func readConfig(configPath string, logLevel string) (config, error) {
	var conf config
	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")
	viper.AddConfigPath(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return conf, errors.Wrap(err, "Read config failed")
	}
	if err := checkConfigFile(); err != nil {
		return conf, errors.Wrap(err, "Check config file failed")
	}

	conf.Timeout = viper.GetDuration("timeout")

	conf.Region = viper.GetString("region")

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
		Queue:      viper.GetString("credentials.rabbit.queue"),
	}
	rabbit.URI = strings.Join([]string{"amqp://", rabbit.User, ":", rabbit.Password, "@", rabbit.Host, "/", rabbit.Vhost}, "")
	conf.Credentials.Rabbit = rabbit

	conf.Workers = viper.GetInt("workers")

	conf.Graphite.Hostname = "graphite-relay.localdomain"
	conf.Graphite.Port = 2003
	conf.Graphite.Prefix = "swift-consometer"

	conf.LogLevel = viper.GetString("log_level")
	if logLevel != "" {
		parsedLogLevel, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return conf, errors.Wrap(err, "Bad log level")
		}
		log.Level = parsedLogLevel
	} else {
		parsedLogLevel, err := logrus.ParseLevel(conf.LogLevel)
		if err != nil {
			return conf, errors.Wrap(err, "Bad log level")
		}
		log.Level = parsedLogLevel
	}
	log.Debug("Config read: ", viper.AllSettings())

	return conf, nil
}
