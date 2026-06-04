package rabbitmq

import (
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"

	"radius-parser/internal/cgnat"
	"radius-parser/internal/session"
	"radius-parser/internal/whitelist"
)

func StartConsumers() error {

	if err := startStatsConsumer(); err != nil {
		return err
	}

	if err := startCGNATConsumer(); err != nil {
		return err
	}

	if err := startWhitelistConsumer(); err != nil {
		return err
	}

	return nil
}

func startStatsConsumer() error {

	q, err := GlobalClient.ch.QueueDeclare(
		RouteSessionStats,
		true,
		false,
		false,
		false,
		amqp.Table{
        "x-queue-type": "quorum",
    },
	)
	if err != nil {
		return err
	}

	err = GlobalClient.ch.QueueBind(
		q.Name,
		RouteSessionStats,
		GlobalClient.cfg.Exchange,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	msgs, err := GlobalClient.ch.Consume(
		q.Name,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {

		for d := range msgs {

			var s StatsMessage

			if err := json.Unmarshal(d.Body, &s); err != nil {
				continue
			}

			session.UpdatePacketCount(
				s.SessionID,
				s.PacketCount,
			)
		}
	}()

	return nil
}

func startCGNATConsumer() error {

	q, err := GlobalClient.ch.QueueDeclare(
		RouteCGNATLoad,
		true,
		false,
		false,
		false,
		amqp.Table{
        "x-queue-type": "quorum",
    },
	)
	if err != nil {
		return err
	}

	err = GlobalClient.ch.QueueBind(
		q.Name,
		RouteCGNATLoad,
		GlobalClient.cfg.Exchange,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	msgs, err := GlobalClient.ch.Consume(
		q.Name,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {

		for d := range msgs {

			var entries []cgnat.CgnatEntry

			if json.Unmarshal(d.Body, &entries) == nil {
				cgnat.LoadFromBytes(entries)
			}
		}
	}()

	return nil
}

func startWhitelistConsumer() error {

	q, err := GlobalClient.ch.QueueDeclare(
		RouteWhitelistLoad,
		true,
		false,
		false,
		false,
		amqp.Table{
        "x-queue-type": "quorum",
    },
	)
	if err != nil {
		return err
	}

	err = GlobalClient.ch.QueueBind(
		q.Name,
		RouteWhitelistLoad,
		GlobalClient.cfg.Exchange,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	msgs, err := GlobalClient.ch.Consume(
		q.Name,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {

		for d := range msgs {

			var entries []whitelist.WhitelistInfo

			if json.Unmarshal(d.Body, &entries) == nil {
				whitelist.LoadFromBytes(entries)
			}
		}
	}()

	return nil
}