package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Notifier struct {
	webhookURL string
	client     *http.Client
	maxRetries int
}

// Message represents a Discord webhook message payload
type Message struct {
	Content string      `json:"content,omitempty"`
	Embeds  []Embed     `json:"embeds,omitempty"`
	Username string     `json:"username,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
}

// Embed represents a Discord embed
type Embed struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Color       int     `json:"color,omitempty"`
	Fields      []Field `json:"fields,omitempty"`
	Timestamp   string  `json:"timestamp,omitempty"`
}

// Field represents a Discord embed field
type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// New creates a new Discord webhook notifier
func New(webhookURL string) (*Notifier, error) {
	if webhookURL == "" {
		return nil, fmt.Errorf("webhook URL cannot be empty")
	}

	// Validate URL
	parsedURL, err := url.Parse(webhookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid webhook URL: %w", err)
	}

	if parsedURL.Scheme == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("webhook URL must use http or https scheme")
	}

	if parsedURL.Host == "" {
		return nil, fmt.Errorf("webhook URL must have a valid host")
	}

	return &Notifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxRetries: 3,
	}, nil
}

// SetMaxRetries sets the maximum number of retry attempts
func (n *Notifier) SetMaxRetries(maxRetries int) {
	n.maxRetries = maxRetries
}

// SendMessage sends a simple text message to the Discord webhook
func (n *Notifier) SendMessage(text string) error {
	return n.SendMessageWithContext(context.Background(), text)
}

// SendMessageWithContext sends a simple text message with context support
func (n *Notifier) SendMessageWithContext(ctx context.Context, text string) error {
	if text == "" {
		return fmt.Errorf("message cannot be empty")
	}

	// Create message payload
	msg := Message{
		Content: text,
	}

	return n.send(ctx, msg)
}

// SendFormattedMessage sends a formatted message with title, description, and fields
func (n *Notifier) SendFormattedMessage(title, description string, fields map[string]string) error {
	return n.SendFormattedMessageWithContext(context.Background(), title, description, fields)
}

// SendFormattedMessageWithContext sends a formatted message with context support
func (n *Notifier) SendFormattedMessageWithContext(ctx context.Context, title, description string, fields map[string]string) error {
	// Truncate title to Discord's limit
	if len(title) > 256 {
		title = title[:256]
	}

	// Truncate description to Discord's limit
	if len(description) > 4096 {
		description = description[:4096]
	}

	embed := Embed{
		Title:       title,
		Description: description,
		Color:       0x5865F2, // Discord's default blurple color
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	// Add fields if provided
	if len(fields) > 0 {
		for key, value := range fields {
			embed.Fields = append(embed.Fields, Field{
				Name:   key,
				Value:  value,
				Inline: true,
			})
		}
	}

	msg := Message{
		Embeds: []Embed{embed},
	}

	return n.send(ctx, msg)
}

// SendWorkBlockApproval sends a formatted message for work block approvals
func (n *Notifier) SendWorkBlockApproval(blockID, title, goal, transition string) error {
	return n.SendWorkBlockApprovalWithContext(context.Background(), blockID, title, goal, transition)
}

// SendWorkBlockApprovalWithContext sends a work block approval message with context support
func (n *Notifier) SendWorkBlockApprovalWithContext(ctx context.Context, blockID, title, goal, transition string) error {
	fields := map[string]string{
		"Block ID":   blockID,
		"Status":    transition,
	}

	return n.SendFormattedMessageWithContext(ctx, title, goal, fields)
}

// send sends the message payload to the Discord webhook with retry logic
func (n *Notifier) send(ctx context.Context, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Check payload size (Discord limit is 8MB but we'll be conservative)
	// 8192 allows for a 4096-character description plus JSON overhead
	if len(payload) > 8192 {
		return fmt.Errorf("message payload too large: %d bytes (max 8192)", len(payload))
	}

	var lastErr error
	for attempt := 0; attempt <= n.maxRetries; attempt++ {
		err := n.doRequest(ctx, payload)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryable(err) {
			return err
		}

		// Don't sleep after the last attempt
		if attempt < n.maxRetries {
			// Exponential backoff: 100ms, 200ms, 400ms, ...
			backoff := time.Duration(100*(attempt+1)) * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("failed to send message after %d retries: %w", n.maxRetries+1, lastErr)
}

// doRequest performs the actual HTTP request to the Discord webhook
func (n *Notifier) doRequest(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", n.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error details
	body, _ := io.ReadAll(resp.Body)

	// Check response status
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("webhook not found (404): invalid webhook URL")
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited (429): %s", string(body))
	case http.StatusBadRequest:
		return fmt.Errorf("bad request (400): %s", string(body))
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized (401): %s", string(body))
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	default:
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}
}

// isRetryable determines if an error should trigger a retry
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Retry on server errors and network issues
	return strings.Contains(errStr, "server error") ||
		strings.Contains(errStr, "request failed") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout")
}
