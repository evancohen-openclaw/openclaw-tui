package gateway

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Client manages the WebSocket connection to an OpenClaw Gateway.
type Client struct {
	url         string
	token       string
	password    string
	version     string
	instanceID  string
	tlsInsecure bool

	conn    *websocket.Conn
	mu      sync.Mutex
	closed  bool
	backoff time.Duration

	pending map[string]chan *ResponseFrame
	pendMu  sync.Mutex

	// Callbacks
	OnHelloOk      func(*HelloOk)
	OnEvent        func(event string, payload json.RawMessage, seq *int)
	OnConnected    func()
	OnDisconnected func(reason string)
	OnReconnecting func(attempt int)
}

// NewClient creates a new gateway client.
func NewClient(wsURL, token, password, version string, tlsInsecure bool) *Client {
	return &Client{
		url:         wsURL,
		token:       token,
		password:    password,
		version:     version,
		tlsInsecure: tlsInsecure,
		instanceID:  uuid.New().String(),
		pending:     make(map[string]chan *ResponseFrame),
		backoff:     time.Second,
	}
}

// Start connects to the gateway with auto-reconnect. Call from a goroutine.
func (c *Client) Start() {
	attempt := 0
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()

		attempt++
		err := c.connectAndRun()
		_ = err // error is passed through OnDisconnected callback

		c.flushPending(fmt.Errorf("disconnected"))

		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()

		// Exponential backoff: 1s, 2s, 4s, 8s, 15s max
		wait := c.backoff
		if c.OnDisconnected != nil {
			c.OnDisconnected(fmt.Sprintf("%v (reconnecting in %s)", err, wait.Round(time.Second)))
		}

		time.Sleep(wait)
		if c.backoff < 15*time.Second {
			c.backoff = time.Duration(float64(c.backoff) * 1.5)
			if c.backoff > 15*time.Second {
				c.backoff = 15 * time.Second
			}
		}

		if c.OnReconnecting != nil {
			c.OnReconnecting(attempt)
		}
	}
}

// Stop closes the connection.
func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) connectAndRun() error {
	// Build WS URL with /ws path
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	u.Path = "/ws"

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	if c.tlsInsecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	defer func() {
		conn.Close()
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
	}()

	// Read challenge event
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read challenge: %w", err)
	}

	var challenge ChallengeFrame
	if err := json.Unmarshal(msg, &challenge); err != nil {
		return fmt.Errorf("parse challenge: %w", err)
	}
	if challenge.Type != "event" || challenge.Event != "connect.challenge" {
		return fmt.Errorf("expected connect.challenge event, got type=%q event=%q", challenge.Type, challenge.Event)
	}

	// Send connect
	connectParams := ConnectParams{
		MinProtocol: 3,
		MaxProtocol: 3,
		Client: ClientInfo{
			ID:          "openclaw-tui",
			DisplayName: "openclaw-tui",
			Version:     c.version,
			Platform:    runtime.GOOS,
			Mode:        "ui",
			InstanceID:  c.instanceID,
		},
		Caps: []string{"tool-events"},
	}

	if c.token != "" || c.password != "" {
		connectParams.Auth = &ConnectAuth{
			Token:    c.token,
			Password: c.password,
		}
	}

	helloRes, err := c.doRequest(conn, "connect", connectParams)
	if err != nil {
		return fmt.Errorf("connect handshake: %w", err)
	}

	var hello HelloOk
	if err := json.Unmarshal(helloRes.Payload, &hello); err != nil {
		return fmt.Errorf("parse hello: %w", err)
	}

	c.backoff = time.Second // reset on success

	if c.OnHelloOk != nil {
		c.OnHelloOk(&hello)
	}
	if c.OnConnected != nil {
		c.OnConnected()
	}

	// Read loop
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var generic GenericFrame
		if err := json.Unmarshal(msg, &generic); err != nil {
			continue
		}

		switch generic.Type {
		case "res":
			var res ResponseFrame
			if err := json.Unmarshal(msg, &res); err != nil {
				continue
			}
			c.resolvePending(res.ID, &res)

		case "event":
			var evt EventFrame
			if err := json.Unmarshal(msg, &evt); err != nil {
				continue
			}
			if c.OnEvent != nil {
				c.OnEvent(evt.Event, evt.Payload, evt.Seq)
			}
		}
	}
}

// Request sends an RPC request and waits for the response.
func (c *Client) Request(method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	res, err := c.doRequest(conn, method, params)
	if err != nil {
		return nil, err
	}

	if !res.OK {
		errMsg := "request failed"
		if res.Error != nil {
			errMsg = res.Error.Message
			if errMsg == "" {
				errMsg = res.Error.Code
			}
		}
		return nil, fmt.Errorf("%s: %s", method, errMsg)
	}

	return res.Payload, nil
}

func (c *Client) doRequest(conn *websocket.Conn, method string, params interface{}) (*ResponseFrame, error) {
	id := uuid.New().String()

	frame := RequestFrame{
		Type:   "req",
		ID:     id,
		Method: method,
		Params: params,
	}

	data, err := json.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ch := make(chan *ResponseFrame, 1)
	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()

	defer func() {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
	}()

	c.mu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case res := <-ch:
		return res, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("request timeout: %s", method)
	}
}

func (c *Client) resolvePending(id string, res *ResponseFrame) {
	c.pendMu.Lock()
	ch, ok := c.pending[id]
	c.pendMu.Unlock()
	if ok {
		ch <- res
	}
}

func (c *Client) flushPending(err error) {
	c.pendMu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.pendMu.Unlock()
}
