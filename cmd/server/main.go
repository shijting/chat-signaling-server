package main

import (
	"log"
	"net"
	"time"

	signaler "github.com/shijting/chat-signaling-server/pkg/impl/signal-server"
	proto "github.com/shijting/chat-signaling-server/pkg/proto/signaling"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

var (
	serverAddr   string
	signalSvrOpt signaler.Options

	cmd = &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			impl, err := signaler.New(signalSvrOpt)
			if err != nil {
				panic(err)
			}
			listener, err := net.Listen("tcp4", serverAddr)
			if err != nil {
				panic(err)
			}
			grpcServer := grpc.NewServer()
			proto.RegisterSignalingServer(grpcServer, impl)
			log.Println("listening on", serverAddr)
			if err := grpcServer.Serve(listener); err != nil {
				panic(err)
			}
		},
	}
)

func main() {
	cmd.Flags().StringVar(&serverAddr, "listen", "0.0.0.0:4444", "")
	cmd.Flags().StringSliceVar(&signalSvrOpt.RedisServers, "redis-server", []string{"127.0.0.1:6379"}, "")
	cmd.Flags().IntVar(&signalSvrOpt.RedisDatabase, "redis-db", 0, "")
	cmd.Flags().StringVar(&signalSvrOpt.RedisKeyPrefix, "redis-key-prefix", "signaling", "")
	cmd.Flags().Int64Var(&signalSvrOpt.NameRandomSeed, "random-seed", time.Now().Unix(), "")

	cmd.Execute()
}
