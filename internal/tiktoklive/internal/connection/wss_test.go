package connection

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"raikiri/internal/tiktoklive/internal/events"
	pb "raikiri/internal/tiktoklive/internal/protocol"
)

func TestProcessFrameDecodesCompressedChat(t *testing.T) {
	chatBytes, err := proto.Marshal(&pb.WebcastChatMessage{
		Common: &pb.Common{MsgId: 42}, User: &pb.User{UniqueId: "viewer"}, Content: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	responseBytes, err := proto.Marshal(&pb.WebcastResponse{Messages: []*pb.WebcastResponse_Message{{
		Method: "WebcastChatMessage", Payload: chatBytes,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(responseBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	frameBytes, err := proto.Marshal(&pb.WebcastPushFrame{PayloadType: "msg", Payload: compressed.Bytes()})
	if err != nil {
		t.Fatal(err)
	}

	eventCh := make(chan events.Event, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := processFrame(context.Background(), frameBytes, nil, eventCh, logger); err != nil {
		t.Fatal(err)
	}
	got := <-eventCh
	if got.Type != events.EventChat {
		t.Fatalf("event type = %v, want chat", got.Type)
	}
	chat, ok := got.Data.(*pb.WebcastChatMessage)
	if !ok || chat.GetCommon().GetMsgId() != 42 || chat.GetUser().GetUniqueId() != "viewer" || chat.GetContent() != "hello" {
		t.Fatalf("unexpected decoded chat: %#v", got.Data)
	}
}

func TestReadLoopStopsPromptlyOnCancellation(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		done <- readLoop(ctx, server, "123", time.Hour, make(chan events.Event), nil, logger)
	}()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("read loop did not stop after cancellation")
	}
}

func TestProcessFrameHonorsCancellationDuringBackpressure(t *testing.T) {
	chatBytes, err := proto.Marshal(&pb.WebcastChatMessage{Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	responseBytes, err := proto.Marshal(&pb.WebcastResponse{Messages: []*pb.WebcastResponse_Message{{Method: "WebcastChatMessage", Payload: chatBytes}}})
	if err != nil {
		t.Fatal(err)
	}
	frameBytes, err := proto.Marshal(&pb.WebcastPushFrame{PayloadType: "msg", Payload: responseBytes})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := processFrame(ctx, frameBytes, nil, make(chan events.Event), logger); err != nil {
		t.Fatal(err)
	}
}
