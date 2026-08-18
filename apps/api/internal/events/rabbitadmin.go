package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// QueueInfo is the subset of RabbitMQ's management API queue detail this
// package exposes to operator surfaces (health, activity backlog).
type QueueInfo struct {
	Messages       int
	MessagesReady  int
	MessagesUnacked int
}

// Admin queries RabbitMQ's management HTTP API (mgmt UI, loopback-only in
// dev per K4-1) for queue depth — the RabbitMQ replacement for asynq's
// Redis-backed Inspector, which had no broker-native equivalent to query.
type Admin struct {
	baseURL  string // e.g. http://localhost:15672
	user     string
	password string
	client   *http.Client
}

// NewAdmin builds an Admin from the same amqp:// URL the publisher/consumer
// dial, assuming the management plugin listens on 15672 on the same host
// (true for both compose files, T001/T002).
func NewAdmin(amqpURL string) (*Admin, error) {
	u, err := url.Parse(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("events: invalid RABBITMQ_URL for admin: %w", err)
	}
	pass, _ := u.User.Password()
	return &Admin{
		baseURL:  fmt.Sprintf("http://%s:15672", u.Hostname()),
		user:     u.User.Username(),
		password: pass,
		client:   &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// QueueDepth returns the message counts for one queue, by name, in the
// default vhost.
func (a *Admin) QueueDepth(queueName string) (QueueInfo, error) {
	endpoint := fmt.Sprintf("%s/api/queues/%%2F/%s", a.baseURL, url.PathEscape(queueName))
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return QueueInfo{}, err
	}
	req.SetBasicAuth(a.user, a.password)

	resp, err := a.client.Do(req)
	if err != nil {
		return QueueInfo{}, fmt.Errorf("events: admin queue depth %s: %w", queueName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return QueueInfo{}, fmt.Errorf("events: admin queue depth %s: status %d", queueName, resp.StatusCode)
	}

	var body struct {
		Messages        int `json:"messages"`
		MessagesReady   int `json:"messages_ready"`
		MessagesUnacked int `json:"messages_unacknowledged"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return QueueInfo{}, fmt.Errorf("events: admin queue depth %s: decode: %w", queueName, err)
	}
	return QueueInfo{Messages: body.Messages, MessagesReady: body.MessagesReady, MessagesUnacked: body.MessagesUnacked}, nil
}
