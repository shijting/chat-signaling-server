package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	signal_server "github.com/shijting/chat-signaling-server/pkg/impl/signal-server"
	proto "github.com/shijting/chat-signaling-server/pkg/proto/signaling"
	"google.golang.org/grpc"
)

func main() {
	flag.Parse()
	port := flag.Arg(0)
	if port == "" {
		port = "6666"
	}
	impl, err := signal_server.New(signal_server.Options{
		RedisServers:   []string{"127.0.0.1:6379"},
		RedisDatabase:  0,
		RedisKeyPrefix: "signaling",
	})
	if err != nil {
		panic(err)
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%s", port))
	if err != nil {
		panic(err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterSignalingServer(grpcServer, impl)
	log.Println("grpc server listening on ", port)
	if err = grpcServer.Serve(listener); err != nil {
		panic(err)
	}
}
