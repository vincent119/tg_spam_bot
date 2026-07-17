package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientDeleteMessage(t *testing.T) {
	t.Parallel()
	var path, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		data := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(data)
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteMessage(context.Background(), 1, 2); err != nil {
		t.Fatal(err)
	}
	if path != "/bottoken/deleteMessage" || !strings.Contains(body, `"message_id":2`) {
		t.Fatalf("path = %s body = %s", path, body)
	}
}

func TestClientMasksTokenFromError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"unauthorized"}`))
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "sensitive-token", server.Client())
	err := client.DeleteMessage(context.Background(), 1, 2)
	if err == nil || strings.Contains(err.Error(), "sensitive-token") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestClientServerAndRateLimitErrors(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":` + http.StatusText(status) + `,"description":"failed"}`))
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, "token", server.Client())
			if err := client.DeleteMessage(context.Background(), 1, 2); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
