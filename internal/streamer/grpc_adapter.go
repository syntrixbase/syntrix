package streamer

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	pb "github.com/syntrixbase/syntrix/api/streamer/v1"
	"google.golang.org/grpc"
)

// grpcStreamAdapter adapts a gRPC stream to work with the streamer service.
// Unlike localStream which uses direct calls, gRPC adapter handles proto messages
// and performs protocol conversion.
type grpcStreamAdapter struct {
	ctx         context.Context
	gatewayID   string
	grpcStream  grpc.BidiStreamingServer[pb.GatewayMessage, pb.StreamerMessage]
	localStream *localStream
	service     *streamerService
	logger      *slog.Logger
}

func newGRPCStreamAdapter(
	ctx context.Context,
	gatewayID string,
	stream grpc.BidiStreamingServer[pb.GatewayMessage, pb.StreamerMessage],
	service *streamerService,
) *grpcStreamAdapter {
	return &grpcStreamAdapter{
		ctx:         ctx,
		gatewayID:   gatewayID,
		grpcStream:  stream,
		localStream: newLocalStream(ctx, gatewayID, service),
		service:     service,
		logger:      service.logger.With("gatewayID", gatewayID),
	}
}

func (g *grpcStreamAdapter) run() error {
	errChan := make(chan error, 2)

	// Handle incoming gRPC messages
	go func() {
		for {
			msg, err := g.grpcStream.Recv()
			if err != nil {
				errChan <- err
				return
			}

			// Process the proto message directly
			if err := g.handleProtoMessage(msg); err != nil {
				g.logger.Error("Failed to handle proto message", "error", err)
				errChan <- err
				return
			}
		}
	}()

	// Handle outgoing events to gRPC
	go func() {
		for {
			select {
			case delivery, ok := <-g.localStream.outgoing:
				if !ok {
					return
				}
				// Convert EventDelivery to proto and send
				protoDelivery := eventDeliveryToProto(delivery)
				protoMsg := &pb.StreamerMessage{
					Payload: &pb.StreamerMessage_Delivery{
						Delivery: protoDelivery,
					},
				}
				if err := g.grpcStream.Send(protoMsg); err != nil {
					errChan <- err
					return
				}
			case <-g.ctx.Done():
				return
			}
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-g.ctx.Done():
		return g.ctx.Err()
	case <-g.service.ctx.Done():
		return g.service.ctx.Err()
	}
}

// handleProtoMessage processes a proto GatewayMessage directly.
func (g *grpcStreamAdapter) handleProtoMessage(msg *pb.GatewayMessage) error {
	switch m := msg.Payload.(type) {
	case *pb.GatewayMessage_Subscribe:
		req := m.Subscribe
		if req.SubscriptionId == "" {
			req.SubscriptionId = uuid.New().String()
		}

		resp, _ := g.service.manager.Subscribe(g.gatewayID, req)

		// Send response back
		return g.grpcStream.Send(&pb.StreamerMessage{
			Payload: &pb.StreamerMessage_SubscribeResponse{
				SubscribeResponse: resp,
			},
		})

	case *pb.GatewayMessage_Unsubscribe:
		if err := g.service.manager.Unsubscribe(m.Unsubscribe.SubscriptionId); err != nil {
			g.logger.Warn("Unsubscribe failed",
				"subscriptionID", m.Unsubscribe.SubscriptionId,
				"error", err)
		}
		return nil

	case *pb.GatewayMessage_Heartbeat:
		return g.grpcStream.Send(&pb.StreamerMessage{
			Payload: &pb.StreamerMessage_HeartbeatAck{
				HeartbeatAck: &pb.HeartbeatAck{
					Timestamp: m.Heartbeat.Timestamp,
				},
			},
		})
	}
	return nil
}
