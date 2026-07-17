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

func TestClientCommandMethods(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bottoken/getMe":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":99,"username":"liyu_spam_bot"}}`))
		case "/bottoken/getChatMember":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"administrator"}}`))
		case "/bottoken/getChat":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"permissions":{"can_send_messages":true}}}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := client.GetMe(t.Context())
	if err != nil || identity.ID != 99 || identity.Username != "liyu_spam_bot" {
		t.Fatalf("GetMe() = %+v, %v", identity, err)
	}
	admin, err := client.IsAdmin(t.Context(), -1001, 1)
	if err != nil || !admin {
		t.Fatalf("IsAdmin() = %v, %v", admin, err)
	}
	for _, call := range []func() error{
		func() error { return client.SendMessage(t.Context(), -1001, 2, "完成") },
		func() error { return client.UnrestrictMember(t.Context(), -1001, 3) },
		func() error { return client.UnbanMember(t.Context(), -1001, 3) },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	if len(paths) != 6 {
		t.Fatalf("API 呼叫數 = %d，預期 6：%v", len(paths), paths)
	}
}

func TestClientMasksTokenFromDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"token sensitive-token invalid"}`))
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "sensitive-token", server.Client())
	err := client.UnbanMember(t.Context(), -1001, 3)
	if err == nil || strings.Contains(err.Error(), "sensitive-token") {
		t.Fatalf("錯誤未遮蔽：%v", err)
	}
}
