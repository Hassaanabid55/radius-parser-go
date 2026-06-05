package rabbitmq

import (
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

func publish(routingKey string, v any) error {

	if GlobalClient == nil {
		return fmt.Errorf("rabbitmq not initialized")
	}

	body, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return GlobalClient.ch.Publish(
		GlobalClient.cfg.Exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func PublishSessionStart(v any) error {
	return publish(RouteSessionStart, v)
}

func PublishSessionStop(v any) error {
	return publish(RouteSessionStop, v)
}

func PublishSessionFinal(v any) error {
	return publish(RouteSessionFinal, v)
}
