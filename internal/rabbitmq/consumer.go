package rabbitmq

import (
	"encoding/json"
	"log"
	"time"

	"radius-parser/internal/cgnat"
	"radius-parser/internal/liveness"
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

	if err := startHeartbeatConsumer(); err != nil {
		return err
	}

	return nil
}

func startStatsConsumer() error {
	msgs, err := GlobalClient.ch.Consume(
		RouteSessionStats,
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

			updated, session := session.UpdatePacketCount(
				s.SessionID,
				s.PacketCount,
				s.ByeSeen,
			)
			if updated {
				PublishSessionFinal(session)
			}
		}
	}()

	return nil
}

func startCGNATConsumer() error {

	msgs, err := GlobalClient.ch.Consume(
		RouteCGNATLoad,
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
			// try array first
			var list []cgnat.CgnatEntry

			if err := json.Unmarshal(d.Body, &list); err == nil {
				cgnat.LoadFromBytes(list)
				continue
			}

			// fallback: single object
			var single cgnat.CgnatEntry
			if err := json.Unmarshal(d.Body, &single); err == nil {
				cgnat.LoadFromBytes([]cgnat.CgnatEntry{single})
				continue
			}

			log.Printf("Failed to parse CGNAT entries")
		}
	}()

	return nil
}

func startWhitelistConsumer() error {

	msgs, err := GlobalClient.ch.Consume(
		RouteWhitelistLoad,
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
			var list []whitelist.WhitelistInfo

			if err := json.Unmarshal(d.Body, &list); err == nil {
				whitelist.LoadFromBytes(list)
				continue
			}

			// fallback: single object
			var single whitelist.WhitelistInfo
			if err := json.Unmarshal(d.Body, &single); err == nil {
				whitelist.LoadFromBytes([]whitelist.WhitelistInfo{single})
				continue
			}
			log.Printf("Failed to parse whitelist entries")
			// try array first
		}
	}()

	return nil
}

func startHeartbeatConsumer() error {

	msgs, err := GlobalClient.ch.Consume(
		RouteHeartbeat,
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

			var hb HeartbeatMessage
			if err := json.Unmarshal(d.Body, &hb); err != nil {
				continue
			}

			liveness.Mu.Lock()
			liveness.Map[hb.NodeID] = time.Now()
			liveness.Mu.Unlock()
		}
	}()

	return nil
}
