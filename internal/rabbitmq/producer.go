package rabbitmq

import (
	"encoding/json"
	"fmt"
	"radius-parser/internal/session"

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
	var start StartSessionMessage = StartSessionMessage{
		AccountSessionID: v.(*session.UserSession).AccountSessionID,
		FramedIPv4:       v.(*session.UserSession).FramedIPv4,
		PublicIPv4:       v.(*session.UserSession).PublicIPv4,
		FramedIPv6:       v.(*session.UserSession).FramedIPv6,
		PortStart:        v.(*session.UserSession).PortStart,
		PortEnd:          v.(*session.UserSession).PortEnd,
		IsWhitelist:      v.(*session.UserSession).IsWhitelist,
		FramedIPv6Len:    v.(*session.UserSession).FramedIPv6Len,
	}
	return publish(RouteSessionStart, start)
}

func PublishSessionStop(v any) error {
	var stop StopSessionMessage = StopSessionMessage{
		AccountSessionID: v.(*session.UserSession).AccountSessionID,
	}
	return publish(RouteSessionStop, stop)
}

func PublishSessionFinal(v any) error {
	return publish(RouteSessionFinal, v)
}
