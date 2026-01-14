package nats

import (
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// SetJetStreamNew allows setting the JetStreamNew variable for testing.
func SetJetStreamNew(f func(nc *nats.Conn) (jetstream.JetStream, error)) func() {
	original := JetStreamNew
	JetStreamNew = f
	return func() {
		JetStreamNew = original
	}
}
