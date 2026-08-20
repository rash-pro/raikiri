package connection

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"google.golang.org/protobuf/proto"
	"raikiri/internal/tiktoklive/internal/events"
	pb "raikiri/internal/tiktoklive/internal/protocol"
	tthttp "raikiri/internal/tiktoklive/internal/web"
)

const (
	defaultHeartbeatInterval = 10 * time.Second
	readBufferSize           = 65536
)

// DeviceBlockedError is returned when the WSS handshake responds with
// Handshake-Msg: DEVICE_BLOCKED, meaning the ttwid was flagged.
type DeviceBlockedError struct{}

func (e *DeviceBlockedError) Error() string {
	return "device blocked — ttwid was flagged, fetch a fresh one"
}

type frameWriter struct {
	conn net.Conn
	mu   sync.Mutex
}

func (w *frameWriter) WriteBinary(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return wsutil.WriteClientBinary(w.conn, data)
}

// RunWebSocket connects to the TikTok Live WSS endpoint and streams events.
// The userAgent parameter sets the User-Agent header for the WSS handshake.
// The cookieHeader is the full Cookie header value (e.g. "ttwid=xxx; sessionid=yyy").
// Pass acceptLanguage for locale-aware header (e.g. "ro-RO,ro;q=0.9"). Empty = auto-detect.
// Pass proxy URL for HTTP CONNECT tunneling; empty falls back to env vars.
func RunWebSocket(ctx context.Context, wssURL string, cookieHeader string, userAgent string, roomID string, staleTimeout time.Duration, acceptLanguage string, proxy string, eventCh chan<- events.Event, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if acceptLanguage == "" {
		lang, reg := tthttp.SystemLocale()
		acceptLanguage = fmt.Sprintf("%s-%s,%s;q=0.9", lang, reg, lang)
	}
	header := http.Header{
		"User-Agent":      {userAgent},
		"Cookie":          {cookieHeader},
		"Origin":          {"https://www.tiktok.com"},
		"Referer":         {"https://www.tiktok.com/"},
		"Accept-Language": {acceptLanguage},
		"Accept-Encoding": {"gzip, deflate"},
		"Cache-Control":   {"no-cache"},
	}

	// Capture Handshake-Msg from non-101 error responses to detect DEVICE_BLOCKED.
	var handshakeMsg string
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP(header),
		OnStatusError: func(status int, reason []byte, resp io.Reader) {
			httpResp, err := http.ReadResponse(bufio.NewReader(resp), nil)
			if err != nil {
				return
			}
			defer httpResp.Body.Close()
			if val := httpResp.Header.Get("Handshake-Msg"); val != "" {
				handshakeMsg = val
			}
		},
	}

	// If an explicit proxy is set, use HTTP CONNECT tunneling via NetDial.
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return fmt.Errorf("wss proxy: invalid URL: %w", err)
		}
		dialer.NetDial = proxyNetDial(proxyURL)
	}

	conn, br, _, err := dialer.Dial(ctx, wssURL)
	if err != nil {
		if strings.EqualFold(handshakeMsg, "DEVICE_BLOCKED") {
			return &DeviceBlockedError{}
		}
		return fmt.Errorf("wss dial: %w", err)
	}
	if br != nil {
		ws.PutReader(br)
	}
	defer conn.Close()
	writes := &frameWriter{conn: conn}

	// send heartbeat + enter room
	hb, err := buildHeartbeat(roomID)
	if err != nil {
		return err
	}
	if err := writes.WriteBinary(hb); err != nil {
		return fmt.Errorf("wss send heartbeat: %w", err)
	}

	enter, err := buildEnterRoom(roomID)
	if err != nil {
		return err
	}
	if err := writes.WriteBinary(enter); err != nil {
		return fmt.Errorf("wss send enter room: %w", err)
	}
	select {
	case eventCh <- events.Event{Type: events.EventConnected, RoomID: roomID}:
	case <-ctx.Done():
		return nil
	}

	// heartbeat goroutine
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(defaultHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				hbBytes, err := buildHeartbeat(roomID)
				if err != nil {
					logger.Warn("heartbeat build failed", "error", err)
					return
				}
				if err := writes.WriteBinary(hbBytes); err != nil {
					logger.Warn("heartbeat send failed", "error", err)
					return
				}
			}
		}
	}()

	err = readLoop(runCtx, conn, roomID, staleTimeout, eventCh, writes, logger)

	// No disconnect emit — client owns lifecycle
	cancel()
	<-heartbeatDone
	return err
}

func readLoop(ctx context.Context, conn net.Conn, roomID string, staleTimeout time.Duration, eventCh chan<- events.Event, writes *frameWriter, logger *slog.Logger) error {
	readDone := make(chan struct{})
	defer close(readDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetReadDeadline(time.Now())
		case <-readDone:
		}
	}()

	for {
		if ctx.Err() != nil {
			return nil
		}

		if err := conn.SetReadDeadline(time.Now().Add(staleTimeout)); err != nil {
			return fmt.Errorf("set read deadline: %w", err)
		}

		msgs, err := wsutil.ReadServerMessage(conn, nil)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logger.Warn("websocket stale", "timeout", staleTimeout)
				return nil
			}
			return fmt.Errorf("wss read: %w", err)
		}

		for _, msg := range msgs {
			if msg.OpCode == ws.OpBinary {
				if err := processFrame(ctx, msg.Payload, writes, eventCh, logger); err != nil {
					logger.Warn("frame processing failed", "error", err)
				}
			}
		}
	}
}

func processFrame(ctx context.Context, data []byte, writes *frameWriter, eventCh chan<- events.Event, logger *slog.Logger) error {
	frame := &pb.WebcastPushFrame{}
	if err := proto.Unmarshal(data, frame); err != nil {
		return fmt.Errorf("unmarshal frame: %w", err)
	}

	switch frame.PayloadType {
	case "msg":
		decompressed, err := DecompressIfGzipped(frame.Payload)
		if err != nil {
			return err
		}
		response := &pb.WebcastResponse{}
		if err := proto.Unmarshal(decompressed, response); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}

		if response.NeedsAck && len(response.InternalExt) > 0 {
			ack, err := buildAck(frame.LogId, response.InternalExt)
			if err == nil {
				if writes == nil {
					return fmt.Errorf("ack requested without a websocket writer")
				}
				if ackErr := writes.WriteBinary(ack); ackErr != nil {
					logger.Warn("ack send failed", "error", ackErr)
				}
			}
		}

		for _, msg := range response.Messages {
			evts := events.Decode(msg.Method, msg.Payload)
			for _, evt := range evts {
				select {
				case eventCh <- evt:
				case <-ctx.Done():
					return nil
				}
			}
		}

	case "im_enter_room_resp":
		// room entry confirmed
	case "hb":
		// heartbeat response
	}

	return nil
}

// proxyNetDial returns a NetDial function that tunnels through an HTTP CONNECT proxy.
// The returned connection is a raw TCP socket after the proxy responds with 200.
func proxyNetDial(proxyURL *url.URL) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		proxyHost := proxyURL.Host
		if !strings.Contains(proxyHost, ":") {
			switch proxyURL.Scheme {
			case "https":
				proxyHost += ":443"
			default:
				proxyHost += ":80"
			}
		}

		d := net.Dialer{}
		proxyConn, err := d.DialContext(ctx, "tcp", proxyHost)
		if err != nil {
			return nil, fmt.Errorf("proxy dial %s: %w", proxyHost, err)
		}

		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
		if proxyURL.User != nil {
			connectReq += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n",
				basicAuth(proxyURL.User))
		}
		connectReq += "\r\n"

		if _, err := proxyConn.Write([]byte(connectReq)); err != nil {
			proxyConn.Close()
			return nil, fmt.Errorf("proxy CONNECT write: %w", err)
		}

		br := bufio.NewReader(proxyConn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			proxyConn.Close()
			return nil, fmt.Errorf("proxy CONNECT response: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			proxyConn.Close()
			return nil, fmt.Errorf("proxy CONNECT failed: HTTP %d", resp.StatusCode)
		}

		return proxyConn, nil
	}
}

// basicAuth encodes proxy credentials as base64 for Proxy-Authorization.
func basicAuth(user *url.Userinfo) string {
	username := user.Username()
	password, _ := user.Password()
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
