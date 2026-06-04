package rabbitmq

import (
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Client struct {
	cfg Config

	conn *amqp.Connection
	ch   *amqp.Channel
}

var (
	GlobalClient *Client
	initOnce     sync.Once
)

func Init(cfg Config) error {

	var initErr error

	initOnce.Do(func() {

		url := fmt.Sprintf(
			"amqp://%s:%s@%s:%d/%s",
			cfg.User,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Vhost,
		)

		conn, err := amqp.Dial(url)
		if err != nil {
			initErr = err
			return
		}

		ch, err := conn.Channel()
		if err != nil {
			conn.Close()
			initErr = err
			return
		}

		err = ch.ExchangeDeclare(
			cfg.Exchange,
			"topic",
			true,
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			ch.Close()
			conn.Close()
			initErr = err
			return
		}

		GlobalClient = &Client{
			cfg:  cfg,
			conn: conn,
			ch:   ch,
		}
	})

	return initErr
}

func Close() {

	if GlobalClient == nil {
		return
	}

	if GlobalClient.ch != nil {
		_ = GlobalClient.ch.Close()
	}

	if GlobalClient.conn != nil {
		_ = GlobalClient.conn.Close()
	}

	GlobalClient = nil
}