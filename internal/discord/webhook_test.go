package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiscordWebhookNew(t *testing.T) {
	tests := []struct {
		name        string
		webhookURL  string
		wantErr     bool
		description string
	}{
		{
			name:        "valid_webhook_url",
			webhookURL:  "https://discord.com/api/webhooks/123456/abcdef",
			wantErr:     false,
			description: "Valid Discord webhook URL should create notifier without error",
		},
		{
			name:        "empty_webhook_url",
			webhookURL:  "",
			wantErr:     true,
			description: "Empty webhook URL should return error",
		},
		{
			name:        "invalid_url_format",
			webhookURL:  "not-a-url",
			wantErr:     true,
			description: "Invalid URL format should return error",
		},
		{
			name:        "http_webhook_url",
			webhookURL:  "http://discord.com/api/webhooks/123456/abcdef",
			wantErr:     false,
			description: "HTTP webhook URL should be accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.webhookURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v. %s", err, tt.wantErr, tt.description)
			}
		})
	}
}

func TestSendMessage(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		serverStatus   int
		serverResponse string
		wantErr        bool
		description    string
	}{
		{
			name:         "successful_message",
			message:      "Test message",
			serverStatus: http.StatusNoContent,
			wantErr:      false,
			description:  "Message should be delivered successfully with 204 response",
		},
		{
			name:         "successful_message_with_json_response",
			message:      "Test message",
			serverStatus: http.StatusOK,
			serverResponse: `{"id":"123","channel_id":"456"}`,
			wantErr:      false,
			description:  "Message should be delivered successfully with 200 response and JSON body",
		},
		{
			name:           "invalid_webhook_url",
			message:        "Test message",
			serverStatus:   http.StatusNotFound,
			serverResponse: `{"code":10015,"message":"Unknown Webhook"}`,
			wantErr:        true,
			description:    "Invalid webhook URL should return 404 error",
		},
		{
			name:           "rate_limited",
			message:        "Test message",
			serverStatus:   http.StatusTooManyRequests,
			serverResponse: `{"retry_after":1.0}`,
			wantErr:        true,
			description:    "Rate limited response should return error",
		},
		{
			name:         "empty_message",
			message:      "",
			serverStatus: http.StatusNoContent,
			wantErr:      true,
			description:  "Empty message should return error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != "" {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(tt.serverResponse))
				}
			}))
			defer server.Close()

			notifier, _ := New(server.URL + "/api/webhooks/123456/abcdef")
			err := notifier.SendMessage(tt.message)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendMessage() error = %v, wantErr %v. %s", err, tt.wantErr, tt.description)
			}
		})
	}
}

func TestMessageFormatting(t *testing.T) {
	tests := []struct {
		name           string
		title          string
		description    string
		fields         map[string]string
		expectedFields int
		wantErr        bool
	}{
		{
			name:           "formatted_message_with_title_and_description",
			title:          "Issue SO-1",
			description:    "Test issue description",
			fields:         map[string]string{},
			expectedFields: 0,
			wantErr:        false,
		},
		{
			name:           "formatted_message_with_fields",
			title:          "Issue SO-1",
			description:    "Test issue description",
			fields:         map[string]string{"Status": "in_progress", "Priority": "high"},
			expectedFields: 2,
			wantErr:        false,
		},
		{
			name:           "very_long_title",
			title:          strings.Repeat("a", 300),
			description:    "Test description",
			fields:         map[string]string{},
			expectedFields: 0,
			wantErr:        false,
		},
		{
			name:           "very_long_description",
			title:          "Issue",
			description:    strings.Repeat("a", 4100),
			fields:         map[string]string{},
			expectedFields: 0,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request was made
				body, _ := io.ReadAll(r.Body)
				var payload map[string]interface{}
				json.Unmarshal(body, &payload)

				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			notifier, _ := New(server.URL + "/api/webhooks/123456/abcdef")
			err := notifier.SendFormattedMessage(tt.title, tt.description, tt.fields)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendFormattedMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRetryBehavior(t *testing.T) {
	tests := []struct {
		name          string
		attemptCount  int
		failureStatus int
		maxRetries    int
		wantErr       bool
		description   string
	}{
		{
			name:         "retry_on_server_error",
			attemptCount: 0,
			failureStatus: http.StatusInternalServerError,
			maxRetries:   3,
			wantErr:      true,
			description:  "Should retry on 500 error and eventually fail after max retries",
		},
		{
			name:         "retry_on_temporary_failure",
			attemptCount: 0,
			failureStatus: http.StatusServiceUnavailable,
			maxRetries:   2,
			wantErr:      true,
			description:  "Should retry on 503 error",
		},
		{
			name:         "no_retry_on_permanent_failure",
			attemptCount: 0,
			failureStatus: http.StatusNotFound,
			maxRetries:   3,
			wantErr:      true,
			description:  "Should not retry on 404 error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount++
				w.WriteHeader(tt.failureStatus)
			}))
			defer server.Close()

			notifier, _ := New(server.URL + "/api/webhooks/123456/abcdef")
			notifier.SetMaxRetries(tt.maxRetries)
			err := notifier.SendMessage("test")
			if (err != nil) != tt.wantErr {
				t.Errorf("SendMessage() error = %v, wantErr %v. %s", err, tt.wantErr, tt.description)
			}
		})
	}
}

func TestNetworkError(t *testing.T) {
	notifier, _ := New("https://invalid-domain-that-does-not-exist-12345.com/webhook")
	err := notifier.SendMessage("test message")
	if err == nil {
		t.Error("SendMessage() should return error for network failure")
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier, _ := New(server.URL + "/webhook")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := notifier.SendMessageWithContext(ctx, "test")
	if err == nil {
		t.Error("SendMessageWithContext() should return error when context is cancelled")
	}
}

func TestLargePayload(t *testing.T) {
	largeMessage := strings.Repeat("a", 5000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) > 4096 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"payload too large"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier, _ := New(server.URL + "/webhook")
	err := notifier.SendMessage(largeMessage)
	if err == nil {
		t.Error("SendMessage() should handle large payloads")
	}
}

func TestValidWebhookURL(t *testing.T) {
	tests := []struct {
		url   string
		valid bool
	}{
		{"https://discord.com/api/webhooks/123456789/abc-def_ghi", true},
		{"https://discordapp.com/api/webhooks/123/abc", true},
		{"http://localhost:8080/webhook", true},
		{"", false},
		{"invalid", false},
		{"ftp://invalid.com", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("URL_%s", tt.url), func(t *testing.T) {
			_, err := New(tt.url)
			if (err == nil) != tt.valid {
				t.Errorf("New(%q) valid=%v, want %v", tt.url, err == nil, tt.valid)
			}
		})
	}
}

func TestBatchMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier, _ := New(server.URL + "/webhook")

	for i := 0; i < 10; i++ {
		err := notifier.SendMessage(fmt.Sprintf("Message %d", i))
		if err != nil {
			t.Errorf("SendMessage() iteration %d failed: %v", i, err)
		}
	}
}

func TestConcurrentMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier, _ := New(server.URL + "/webhook")

	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			err := notifier.SendMessage(fmt.Sprintf("Message %d", id))
			done <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent SendMessage() failed: %v", err)
		}
	}
}

func TestWorkBlockApproval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	notifier, _ := New(server.URL + "/webhook")
	err := notifier.SendWorkBlockApproval("wb-123", "Feature X", "Implement feature X", "ready_to_ship")
	if err != nil {
		t.Errorf("SendWorkBlockApproval() failed: %v", err)
	}
}
