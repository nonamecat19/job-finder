package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type QueueInfo struct {
	Messages        int
	MessagesReady   int
	MessagesUnacked int
}

type Admin struct {
	baseURL  string
	user     string
	password string
	client   *http.Client
}

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
