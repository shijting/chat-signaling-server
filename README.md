非常简易用来练手的命令行聊天工具

# Quick start

- 启动一个redis，不需要存储，redis主要是用pubsub
  ```bash
  docker run -d -p 6379:6379 redis
  ```
- 启动服务器
  ```bash
  go run ./cmd/server
  ```
- 运行多个客户端
  ```bash
  go run ./cmd/client 房间名字 客户端名字
  ```

# 没有做的地方

- 身份认证：就是个练手项目要什么身份认证。需要了自己参考 https://grpc.io/docs/guides/auth/ 去做身份认证
- 配置项或者 flag：就是个练手项目，就简单一点吧。需要了自己用环境变量或者 flag 或者 cobra/viper 来做配置吧

# 项目组成

- [pkg/proto/signaling](pkg/proto/signaling): 信令协议的部分，就是非常简单的交换信息
- [pkg/impl/signal-server](pkg/impl/signal-server): 协议的服务器实现部分，具体来说依赖一个 redis的 stun
- [cmd/signal-server](cmd/signal-server): 服务器端的命令，写死了很多东西。想要自己用可以自己加 cobra 啊 viper 啊诸如此类的东西。
- [cmd/demo-signal-client](cmd/demo-signal-client): 客户端命令，依赖我开的一个 coturn/stun 服务，你也可以自己开一个，然后换掉里面的 stun:。使用了 bubbletea，没有维护状态，只维护了名字到连接的一张表，对端有连接就能发，没有连接就发不了。