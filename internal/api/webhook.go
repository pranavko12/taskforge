package api

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

const WebhookDeliverJobType = "webhook.deliver"

type WebhookDeliverPayload struct {
	URL         string            `json:"url"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        json.RawMessage   `json:"body"`
	ContentType string            `json:"contentType,omitempty"`
	TimeoutMS   int               `json:"timeoutMs,omitempty"`
}

func validateWebhookDeliverPayload(raw json.RawMessage) error {
	var p WebhookDeliverPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return errors.New("invalid payload")
	}

	p.URL = strings.TrimSpace(p.URL)
	p.Method = strings.ToUpper(strings.TrimSpace(p.Method))
	p.ContentType = strings.TrimSpace(p.ContentType)

	if p.URL == "" {
		return errors.New("payload.url is required")
	}

	u, err := url.ParseRequestURI(p.URL)
	if err != nil {
		return errors.New("payload.url is invalid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("payload.url must use http or https")
	}
	if u.Host == "" {
		return errors.New("payload.url must include host")
	}

	if p.Method == "" {
		p.Method = "POST"
	}
	if p.Method != "POST" {
		return errors.New("payload.method must be POST")
	}

	if len(p.Body) == 0 {
		return errors.New("payload.body is required")
	}

	if p.ContentType == "" {
		p.ContentType = "application/json"
	}

	if p.TimeoutMS == 0 {
		p.TimeoutMS = 3000
	}
	if p.TimeoutMS < 100 || p.TimeoutMS > 10000 {
		return errors.New("payload.timeoutMs out of range")
	}

	if len(p.Headers) > 50 {
		return errors.New("payload.headers too many entries")
	}
	for k, v := range p.Headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			return errors.New("payload.headers has empty key")
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
