package signal_server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"

	proto "github.com/shijting/chat-signaling-server/pkg/proto/signaling"

	"github.com/goombaio/namegenerator"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

type SignalingServer struct {
	proto.UnimplementedSignalingServer

	names namegenerator.Generator

	redisKeyPrefix string
	redis          redis.UniversalClient
}

type Options struct {
	RedisServers  []string
	RedisDatabase int

	RedisKeyPrefix string

	NameRandomSeed int64
}

func New(options Options) (*SignalingServer, error) {
	redisClient := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: options.RedisServers,
		DB:    options.RedisDatabase,
	})

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	return &SignalingServer{
		redis:          redisClient,
		redisKeyPrefix: options.RedisKeyPrefix,

		names: namegenerator.NewNameGenerator(options.NameRandomSeed),
	}, nil
}

func (signalingServer SignalingServer) handleStream(ctx context.Context, name string, errGroup *errgroup.Group, stream proto.Signaling_BiuServer) func() error {
	return func() error {
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return nil
			} else if err == nil {
				switch innerMsg := msg.Message.(type) {
				case *proto.SignalingMessage_Bootstrap:
					errGroup.Go(signalingServer.handleRedisPubSub(ctx, name, msg.Room, stream))
				case *proto.SignalingMessage_DiscoverRequest:
					// ignore msg.Receiver, from sender to whole channel
					if received, err := signalingServer.redis.Publish(ctx, signalingServer.redisKeyPrefix+":"+msg.Room+":discover", msg.Sender).Result(); err != nil {
						return err
					} else {
						log.Printf("peers received discover request %v -> %v(all): %v", name, msg.Room, received)
					}
				case *proto.SignalingMessage_DiscoverResponse:
					if received, err := signalingServer.redis.Publish(ctx, signalingServer.redisKeyPrefix+":"+msg.Room+":discover:"+*msg.Receiver, msg.Sender).Result(); err != nil {
						return err
					} else {
						log.Printf("peers received discover response %v -> %v(%v): %v", name, msg.Room, *msg.Receiver, received)
					}
				case *proto.SignalingMessage_SessionOffer:
					payload := &bytes.Buffer{}
					if err := json.NewEncoder(payload).Encode(innerMsg.SessionOffer); err != nil {
						return err
					}

					if received, err := signalingServer.redis.Publish(ctx, signalingServer.redisKeyPrefix+":"+msg.Room+":offer:"+*msg.Receiver, payload.String()).Result(); err != nil {
						return err
					} else {
						log.Printf("peers received discover response: %v", received)
					}
				case *proto.SignalingMessage_SessionAnswer:
					payload := &bytes.Buffer{}
					if err := json.NewEncoder(payload).Encode(innerMsg.SessionAnswer); err != nil {
						return err
					}

					if received, err := signalingServer.redis.Publish(ctx, signalingServer.redisKeyPrefix+":"+msg.Room+":answer:"+*msg.Receiver, payload.String()).Result(); err != nil {
						return err
					} else {
						log.Printf("peers received discover response: %v", received)
					}
				}
			} else {
				return err
			}
		}
	}
}

func (signalingServer SignalingServer) handleRedisPubSub(ctx context.Context, name, room string, stream proto.Signaling_BiuServer) func() error {
	return func() error {
		pubsub := signalingServer.redis.Subscribe(ctx,
			signalingServer.redisKeyPrefix+":"+room+":discover",
			signalingServer.redisKeyPrefix+":"+room+":discover:"+name,
			signalingServer.redisKeyPrefix+":"+room+":offer:"+name,
			signalingServer.redisKeyPrefix+":"+room+":answer:"+name,
		)
		defer pubsub.Unsubscribe(ctx)
		defer pubsub.Close()

		if err := stream.Send(&proto.SignalingMessage{
			Room:    room,
			Sender:  name,
			Message: &proto.SignalingMessage_Bootstrap{},
		}); err != nil {
			return err
		}

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return nil
			case msg := <-ch:

				switch msg.Channel {
				case signalingServer.redisKeyPrefix + ":" + room + ":discover":
					if err := stream.Send(&proto.SignalingMessage{
						Room:    room,
						Sender:  msg.Payload,
						Message: &proto.SignalingMessage_DiscoverRequest{},
					}); err != nil {
						return err
					}
				case signalingServer.redisKeyPrefix + ":" + room + ":discover:" + name:
					if msg.Payload == name {
						continue
					}
					if err := stream.Send(&proto.SignalingMessage{
						Room:     room,
						Sender:   msg.Payload,
						Receiver: &name,
						Message:  &proto.SignalingMessage_DiscoverResponse{},
					}); err != nil {
						return err
					}
				case signalingServer.redisKeyPrefix + ":" + room + ":offer:" + name:
					sdpMessage := &proto.SDPMessage{}
					if err := json.NewDecoder(strings.NewReader(msg.Payload)).Decode(sdpMessage); err != nil {
						return err
					}

					if err := stream.Send(&proto.SignalingMessage{
						Room:     room,
						Sender:   sdpMessage.Sender,
						Receiver: &name,
						Message: &proto.SignalingMessage_SessionOffer{
							SessionOffer: sdpMessage,
						},
					}); err != nil {
						return err
					}
				case signalingServer.redisKeyPrefix + ":" + room + ":answer:" + name:
					sdpMessage := &proto.SDPMessage{}
					if err := json.NewDecoder(strings.NewReader(msg.Payload)).Decode(sdpMessage); err != nil {
						return err
					}

					if err := stream.Send(&proto.SignalingMessage{
						Room:     room,
						Sender:   sdpMessage.Sender,
						Receiver: &name,
						Message: &proto.SignalingMessage_SessionAnswer{
							SessionAnswer: sdpMessage,
						},
					}); err != nil {
						return err
					}
				}
			}
		}
	}
}

func (signalingServer SignalingServer) Biu(stream proto.Signaling_BiuServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	name := signalingServer.names.Generate()

	errGroup := &errgroup.Group{}

	errGroup.Go(signalingServer.handleStream(ctx, name, errGroup, stream))

	return errGroup.Wait()
}
