package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/rackspace/gophercloud"
	"github.com/spf13/viper"
	"strings"
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
		"regions",
		"ticker",
		"log_level"}

	for _, key := range mandatoryKeys {
		if !viper.IsSet(key) {
			return errors.New(fmt.Sprintf("Incomplete configuration. Missing key %s", key))
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
	Regions  []string
	Ticker   int
	LogLevel string
}

func readConfig(configPath string, logLevel string) (config, error) {
	viper.SetConfigType("yaml")
	viper.SetConfigName("consometer")
	viper.AddConfigPath(configPath)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Read config failed")
	}
	err := checkConfigFile()
	if err != nil {
		return nil, errors.Wrap(err, "Check config file failed")
	}

	var conf config
	conf.Regions = viper.GetStringSlice("regions")

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

	conf.Ticker = viper.GetInt("ticker")

	conf.LogLevel = viper.GetString("log_level")
	if logLevel != "" {
		parsedLogLevel, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return nil, errors.Wrap(err, "Bad log level")
		}
		log.Level = parsedLogLevel
	} else {
		parsedLogLevel, err := logrus.ParseLevel(conf.LogLevel)
		if err != nil {
			return nil, errors.Wrap(err, "Bad log level")
		}
		log.Level = parsedLogLevel
	}
	log.Debug("Config read: ", viper.AllSettings())

	return conf, nil
}
