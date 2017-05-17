package main

import (
	"encoding/json"
	"strconv"

	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

type rabbitPayload struct {
	Args struct {
		Data []AccountInfo `json:"data"`
	} `json:"args"`
}

func fakeSetupRabbit() (chan []AccountInfo, chan int, error) {
	input := make(chan []AccountInfo)
	confirm := make(chan int, 1)
	go func() {
		defer close(confirm)
		for ais := range input {
			var size int64
			for _, a := range ais {
				conso, err := strconv.ParseInt(a.CounterVolume, 10, 64)
				if err != nil {
					return
				}
				size += conso
			}
			log.Debugf("Publishing %v Accounts of total size %v\n", len(ais), size)
			confirm <- len(ais)
		}
	}()
	return input, confirm, nil
}

func setupRabbit(rabbit rabbitCreds) (chan []AccountInfo, chan int, error) {
	log.Debug("Connecting to: ", rabbit.URI)
	conn, err := amqp.Dial(rabbit.URI)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to connect to RabbitMQ")
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to open channel")
	}

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
		return nil, nil, errors.Wrap(err, "Failed declaring exchange")
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
		return nil, nil, errors.Wrap(err, "Failed declaring queue")
	}

	log.Debug("Binding queue to exchange")
	if err := ch.QueueBind(
		rabbit.Queue,      // name of the queue
		rabbit.RoutingKey, // bindingKey
		rabbit.Exchange,   // sourceExchange
		false,             // noWait
		nil,               // arguments
	); err != nil {
		return nil, nil, errors.Wrap(err, "Failed binding queue")
	}

	input := make(chan []AccountInfo)
	confirm := make(chan int, 1)

	go DeliverPayloads(rabbit, conn, ch, input, confirm)

	return input, confirm, nil
}

func DeliverPayloads(rabbit rabbitCreds, conn *amqp.Connection, ch *amqp.Channel, msgChan <-chan []AccountInfo, confirm chan int) {
	defer ch.Close()     // clean-up
	defer conn.Close()   // clean-up
	defer close(confirm) // this signals the outer routine that job is done/canceled
	for ais := range msgChan {
		size := len(ais)
		output := rabbitPayload{}
		output.Args.Data = ais
		rbMsg, err := json.Marshal(output)
		if err != nil {
			log.Errorf("cannot parse rabbit payload: %v", err)
		}

		err = ch.Publish(
			rabbit.Exchange,   // exchange
			rabbit.RoutingKey, // routing key
			false,             // mandatory
			false,             // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        rbMsg,
			})
		if err != nil {
			log.Errorf("Failed to publish message: %v", err)
		} else {
			confirm <- size
		}
	}
}
