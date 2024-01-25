package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	proto "github.com/shijting/chat-signaling-server/pkg/proto/signaling"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pion/webrtc/v3"
)

type errMsg error

type peerMsg struct {
	Peer    string
	Message string
}

type systemMsg string

type SignalClientUI struct {
	viewport viewport.Model
	textarea textarea.Model
}

var (
	StyleSender       = lipgloss.NewStyle()
	StyleSenderBold   = StyleSender.Bold(true)
	StyleError        = lipgloss.NewStyle().Foreground(lipgloss.Color("red"))
	StylePeer         = lipgloss.NewStyle()
	StylePeerSelected = lipgloss.NewStyle().Background(lipgloss.Color("gray"))
)

type SignalClient struct {
	UI      SignalClientUI
	Program *tea.Program

	webrtcConfig webrtc.Configuration

	Room string
	Name string

	Messages []string
	Error    error

	Ready bool

	MessageIsValid bool
	Message        string
	Receiver       string

	PeerConns map[string]*webrtc.PeerConnection
	Channels  map[string]*webrtc.DataChannel
	Lock      sync.Locker
}

func New(room, name string) *SignalClient {
	ta := textarea.New()
	ta.Prompt = "Send a message"
	ta.Focus()

	ta.Prompt = " ! "
	ta.ShowLineNumbers = false
	ta.SetHeight(1)

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	vp := viewport.New(30, 5)
	vp.SetContent(`Welcome to the chat room!
Type a message and press Enter to send.`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	client := &SignalClient{
		UI: SignalClientUI{
			viewport: vp,
			textarea: ta,
		},

		webrtcConfig: webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:nhz.jeffthecoder.xyz:3478", "stun:nhz.jeffthecoder.xyz:3479"},
				},
			},
		},

		Room: room,
		Name: name,

		PeerConns: make(map[string]*webrtc.PeerConnection),
		Channels:  make(map[string]*webrtc.DataChannel),
		Lock:      &sync.Mutex{},
	}

	return client
}

func (client *SignalClient) Init() tea.Cmd {
	go client.ConnectServer(context.Background())
	return textarea.Blink
}

func (client *SignalClient) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	client.UI.textarea, tiCmd = client.UI.textarea.Update(msg)
	client.UI.viewport, vpCmd = client.UI.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc, tea.KeyCtrlD:
			fmt.Println()
			return client, tea.Quit
		case tea.KeyEnter:
			if client.MessageIsValid {
				client.Channels[client.Receiver].SendText(client.Message)

				client.Messages = append(client.Messages, StyleSenderBold.Render("You -> "+client.Receiver+": ")+client.Message)
				client.UI.viewport.SetContent(strings.Join(client.Messages, "\n"))
				client.UI.textarea.Reset()
				client.UI.viewport.GotoBottom()
			}
		}
	case tea.WindowSizeMsg:
		client.UI.textarea.SetWidth(msg.Width)
		client.UI.viewport.Width = msg.Width
		client.UI.viewport.Height = msg.Height - 1

	// We handle errors just like any other message
	case errMsg:
		client.Error = msg
		return client, nil
	case peerMsg:
		client.Messages = append(client.Messages, StyleSenderBold.Render(msg.Peer+" -> You: ")+msg.Message)
		client.UI.viewport.SetContent(strings.Join(client.Messages, "\n"))
		client.UI.viewport.GotoBottom()
	case systemMsg:
		client.Messages = append(client.Messages, string(msg))
		client.UI.viewport.SetContent(strings.Join(client.Messages, "\n"))
		client.UI.viewport.GotoBottom()
	}

	if selected, message, ok := strings.Cut(client.UI.textarea.Value(), ">"); ok {
		selected = strings.TrimSpace(selected)
		message = strings.TrimLeft(message, " \t")
		if _, ok := client.PeerConns[selected]; ok {
			client.MessageIsValid = true
			client.Message = message
			client.Receiver = selected
		} else {
			client.MessageIsValid = false
		}
	} else {
		client.MessageIsValid = false
	}

	if client.MessageIsValid {
		client.UI.textarea.Prompt = " | "
	} else {
		client.UI.textarea.Prompt = " ! "
	}

	return client, tea.Batch(tiCmd, vpCmd)
}

func (client *SignalClient) View() string {
	return fmt.Sprint(
		client.UI.viewport.View()+"\n",
		StyleError.String()+client.UI.textarea.View()+"\n",
	)
}

func (client *SignalClient) ConnectServer(ctx context.Context) {
	client.Program.Send(systemMsg("Dialing to server..."))
	grpcClient, err := grpc.Dial("127.0.0.1:4444", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}

	client.Program.Send(systemMsg("Connecting to room..."))
	signal_server := proto.NewSignalingClient(grpcClient)
	stream, err := signal_server.Biu(context.Background())
	if err != nil {
		panic(err)
	}
	client.Program.Send(systemMsg("Connected."))

	go client.HandleConnection(ctx, grpcClient, stream)
}

func (client *SignalClient) HandleConnection(ctx context.Context, grpcClient *grpc.ClientConn, stream proto.Signaling_BiuClient) {
	defer grpcClient.Close()
	room := client.Room
	clientId := client.Name

	client.Program.Send(systemMsg("Waiting for server to be bootstrapped."))

	stream.Send(&proto.SignalingMessage{
		Room:    room,
		Sender:  clientId,
		Message: &proto.SignalingMessage_Bootstrap{},
	})

	client.Program.Send(systemMsg("Bootstrapped."))

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		switch inner := msg.Message.(type) {
		case *proto.SignalingMessage_Bootstrap:
			client.OnBootstrapReady(ctx, stream, room, clientId)
		case *proto.SignalingMessage_DiscoverRequest:
			client.OnDiscoverRequest(ctx, stream, room, clientId, msg.Sender)
		case *proto.SignalingMessage_DiscoverResponse:
			client.OnDiscoverResponse(ctx, stream, room, clientId, msg.Sender)
		case *proto.SignalingMessage_SessionOffer:
			client.OnOffer(ctx, stream, room, clientId, msg.Sender, inner.SessionOffer.SDP)
		case *proto.SignalingMessage_SessionAnswer:
			client.OnAnswer(ctx, msg.Sender, inner.SessionAnswer.SDP)
		}
	}
}

func (client *SignalClient) OnBootstrapReady(ctx context.Context, stream proto.Signaling_BiuClient, room, name string) {
	client.Program.Send(systemMsg("Server ready!"))
	stream.Send(&proto.SignalingMessage{
		Room:    room,
		Sender:  name,
		Message: &proto.SignalingMessage_DiscoverRequest{},
	})
}

func (client *SignalClient) OnDiscoverRequest(ctx context.Context, stream proto.Signaling_BiuClient, room, name, sender string) {
	client.Program.Send(systemMsg("Client " + sender + " is joining into the room " + room))
	stream.Send(&proto.SignalingMessage{
		Room:     room,
		Sender:   name,
		Receiver: &sender,
		Message:  &proto.SignalingMessage_DiscoverResponse{},
	})
}

func (client *SignalClient) OnDiscoverResponse(ctx context.Context, stream proto.Signaling_BiuClient, room, name, sender string) {
	client.Program.Send(systemMsg("Client " + sender + " ponged"))

	peerConnection, err := client.GetOrCreatePeerConnection(sender)
	if err != nil {
		return
	}
	dataChannel, err := peerConnection.CreateDataChannel("chan", nil)
	if err != nil {
		client.Program.Send(systemMsg(fmt.Sprint("failed to create answer: ", err)))
		return
	}
	client.Channels[sender] = dataChannel
	client.SetupDataChannel(peerConnection, dataChannel, sender)

	sdp, err := peerConnection.CreateOffer(&webrtc.OfferOptions{})
	if err != nil {
		client.Program.Send(systemMsg(fmt.Sprint("Failed to create offer for peer "+sender+": ", err)))
		peerConnection.Close()
		return
	}
	peerConnection.SetLocalDescription(sdp)

	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(sdp); err != nil {
		client.Program.Send(systemMsg(fmt.Sprint("Failed to encode offer for peer "+sender+": ", err)))
		peerConnection.Close()
		return
	}

	client.PeerConns[sender] = peerConnection

	stream.Send(&proto.SignalingMessage{
		Room:     room,
		Sender:   name,
		Receiver: &sender,
		Message: &proto.SignalingMessage_SessionOffer{
			SessionOffer: &proto.SDPMessage{
				SDP:    buffer.String(),
				Type:   proto.SDPMessageType_Data,
				Sender: name,
			},
		},
	})
}

func (client *SignalClient) OnOffer(ctx context.Context, stream proto.Signaling_BiuClient, room, name, sender, sdp string) {
	client.Program.Send(systemMsg("Client " + sender + " is offering"))

	peerConnection, err := client.GetOrCreatePeerConnection(sender)
	if err != nil {
		return
	}
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(strings.NewReader(sdp)).Decode(&offer); err != nil {
		client.Program.Send(systemMsg(fmt.Sprint("Failed to decode offer for peer"+sender+": ", err)))
		return
	}
	peerConnection.SetRemoteDescription(offer)

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		client.Program.Send(systemMsg(fmt.Sprint("Failed to create answer for peer "+sender+": ", err)))
		return
	}
	peerConnection.SetLocalDescription(answer)

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	<-gatherComplete

	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(peerConnection.LocalDescription()); err != nil {
		client.Program.Send(systemMsg(fmt.Sprint("Failed to encode answer for peer"+sender+": ", err)))
		return
	}

	stream.Send(&proto.SignalingMessage{
		Room:     room,
		Sender:   name,
		Receiver: &sender,
		Message: &proto.SignalingMessage_SessionAnswer{
			SessionAnswer: &proto.SDPMessage{
				SDP:    buffer.String(),
				Type:   proto.SDPMessageType_Data,
				Sender: name,
			},
		},
	})
}

func (client *SignalClient) OnAnswer(ctx context.Context, sender, sdp string) {
	client.Program.Send(systemMsg("Client " + sender + " has answered the offer"))

	peerConnection, ok := client.PeerConns[sender]
	if !ok {
		return
	}
	var answer webrtc.SessionDescription
	if err := json.NewDecoder(strings.NewReader(sdp)).Decode(&answer); err != nil {
		client.Program.Send(systemMsg(fmt.Sprint("Failed to decode answer for peer"+sender+": ", err)))
		return
	}
	peerConnection.SetRemoteDescription(answer)

}

func (client SignalClient) GetOrCreatePeerConnection(sender string) (*webrtc.PeerConnection, error) {
	client.Lock.Lock()
	defer client.Lock.Unlock()

	peerConnection, ok := client.PeerConns[sender]
	if ok {
		return peerConnection, nil
	}

	peerConnection, err := webrtc.NewPeerConnection(client.webrtcConfig)
	if err != nil {
		return nil, err
	}
	client.PeerConns[sender] = peerConnection

	closeOnceAndNoMore := &sync.Once{}

	peerConnection.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		if pcs == webrtc.PeerConnectionStateDisconnected || pcs == webrtc.PeerConnectionStateClosed || pcs == webrtc.PeerConnectionStateFailed {

			closeOnceAndNoMore.Do(func() {
				if peerConnection != nil {
					client.Program.Send(systemMsg(fmt.Sprint("Closing connection with ", sender, " for state changed to: ", pcs.String())))
					peerConnection.Close()
					delete(client.Channels, sender)
					delete(client.PeerConns, sender)

					peerConnection = nil
				}
			})
		}
	})

	peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) {
		client.Program.Send(systemMsg(fmt.Sprint("Connected to client " + sender + ": " + dc.Label())))
		client.Channels[sender] = dc

		client.SetupDataChannel(peerConnection, dc, sender)
	})

	return peerConnection, nil
}

func (client *SignalClient) SetupDataChannel(pc *webrtc.PeerConnection, dc *webrtc.DataChannel, sender string) {
	dc.OnOpen(func() {
		client.Program.Send(systemMsg(fmt.Sprint("Channel opened: ", sender)))
	})

	dc.OnMessage(func(dcMsg webrtc.DataChannelMessage) {
		if dcMsg.IsString {
			client.Program.Send(peerMsg{
				Peer:    sender,
				Message: string(dcMsg.Data),
			})
		}
	})
}

func main() {
	flag.Parse()

	if flag.NArg() != 2 {
		panic("invalid usage")
	}

	signalClient := New(flag.Arg(0), flag.Arg(1))

	signalClient.Program = tea.NewProgram(signalClient)

	if _, err := signalClient.Program.Run(); err != nil {
		panic("err")
	}
}
