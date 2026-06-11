package rabbitmq

const (
	RouteSessionStart = "session.start"
	RouteSessionStop  = "session.stop"
	RouteSessionFinal = "session.final"

	RouteSessionStats  = "session.stats"
	RouteCGNATLoad     = "bootstrap.cgnat"
	RouteWhitelistLoad = "bootstrap.whitelist"

	RouteHeartbeat = "node.heartbeat"
)
