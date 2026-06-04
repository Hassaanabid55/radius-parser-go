package rabbitmq

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Vhost    string
	Exchange string
}