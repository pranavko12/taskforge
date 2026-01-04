package api

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

const WebhookDeliverJobType = "webhook.deliver"

type WebhookDeliverPayload struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body"`
}

func validateWebhookDeliverPayload(raw json.RawMessage) error {
	var p WebhookDeliverPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return errors.New("payload is not valid json")
	}

	p.URL = strings.TrimSpace(p.URL)
	if p.URL == "" {
		return errors.New("payload.url is required")
	}

	u, err := url.Parse(p.URL)
	if err != nil {
		return errors.New("payload.url is invalid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("payload.url scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("payload.url host is required")
	}

	method := strings.TrimSpace(p.Method)
	if method == "" {
		method = "POST"
	}
	method = strings.ToUpper(method)
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return errors.New("payload.method must be one of POST, PUT, PATCH, DELETE")
	}

	if len(p.Body) == 0 {
		return errors.New("payload.body is required")
	}
	if len(p.Body) > 256*1024 {
		return errors.New("payload.body too large")
	}

	if len(p.Headers) > 64 {
		return errors.New("payload.headers too many entries")
	}
	for k, v := range p.Headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		if k == "" {
			return errors.New("payload.headers contains empty key")
		}
		if len(k) > 64 {
			return errors.New("payload.headers key too long")
		}
		if len(v) > 512 {
			return errors.New("payload.headers value too long")
		}

		switch strings.ToLower(k) {
		case "host", "content-length", "connection", "transfer-encoding":
			return errors.New("payload.headers contains disallowed header")
		}
	}

	return nil
}
