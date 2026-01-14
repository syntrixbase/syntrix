package nats

import (
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStreamNew is a variable to allow mocking in tests.
var JetStreamNew = func(nc *nats.Conn) (jetstream.JetStream, error) {
	return jetstream.New(nc)
}
