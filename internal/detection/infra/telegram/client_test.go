package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = fmt.Fprintf(w, `{"ok":false,"error_code":%d,"description":"failed"}`, status)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, "token", server.Client())
			err := client.DeleteMessage(context.Background(), 1, 2)
			if err == nil {
				t.Fatal("expected error")
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("錯誤型別 = %T，預期 *APIError", err)
			}
			wantRetryable := status == http.StatusInternalServerError || status == http.StatusTooManyRequests
			if apiErr.IsRetryable() != wantRetryable {
				t.Fatalf("IsRetryable() = %v，預期 %v", apiErr.IsRetryable(), wantRetryable)
			}
			if status == http.StatusForbidden && apiErr.ErrorCode() != "permission_denied" {
				t.Fatalf("ErrorCode() = %q", apiErr.ErrorCode())
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
			_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"administrator","can_delete_messages":true,"can_restrict_members":true}}`))
		case "/bottoken/getChat":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"permissions":{"can_send_messages":true}}}`))
		case "/bottoken/getChatAdministrators":
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"user":{"id":7}},{"user":{"id":8}}]}`))
		case "/bottoken/getWebhookInfo":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"url":"https://example.com/telegram/webhook","pending_update_count":2}}`))
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
	webhook, err := client.GetWebhookInfo(t.Context())
	if err != nil || webhook.URL != "https://example.com/telegram/webhook" || webhook.PendingUpdateCount != 2 {
		t.Fatalf("GetWebhookInfo()=%+v, %v", webhook, err)
	}
	permissions, err := client.BotPermissions(t.Context(), -1001, 99)
	if err != nil || !permissions.CanDeleteMessages || !permissions.CanRestrictMembers {
		t.Fatalf("BotPermissions()=%+v, %v", permissions, err)
	}
	for _, call := range []func() error{
		func() error { return client.SendWarning(t.Context(), -1001, 3, "警告") },
		func() error { return client.SendMessage(t.Context(), -1001, 2, "完成") },
		func() error { return client.RestrictMember(t.Context(), -1001, 3, time.Now().Add(time.Minute)) },
		func() error { return client.UnrestrictMember(t.Context(), -1001, 3) },
		func() error { return client.BanMember(t.Context(), -1001, 3) },
		func() error { return client.UnbanMember(t.Context(), -1001, 3) },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	admins, err := client.AdminIDs(t.Context(), -1001)
	if err != nil || len(admins) != 2 || admins[0] != 7 {
		t.Fatalf("AdminIDs()=%v, %v", admins, err)
	}
	if len(paths) != 12 {
		t.Fatalf("API 呼叫數 = %d，預期 12：%v", len(paths), paths)
	}
}

func TestClientBotPermissionsTreatsCreatorAsAllowed(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"creator"}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	permissions, err := client.BotPermissions(t.Context(), -1001, 99)
	if err != nil {
		t.Fatal(err)
	}
	if !permissions.CanDeleteMessages || !permissions.CanRestrictMembers {
		t.Fatalf("creator 應視為具備必要權限：%+v", permissions)
	}
}

func TestNewClientValidation(t *testing.T) {
	t.Parallel()
	if _, err := NewClient("https://api.telegram.org", "", nil); err == nil {
		t.Fatal("空 Token 應失敗")
	}
	client, err := NewClient("https://api.telegram.org/", "token", nil)
	if err != nil || client.baseURL != "https://api.telegram.org" || client.http == nil {
		t.Fatalf("client=%+v err=%v", client, err)
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

func TestClientMasksURLCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"upstream https://admin:secret@example.com failed"}`))
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "token", server.Client())
	err := client.DeleteMessage(t.Context(), -1001, 3)
	if err == nil || strings.Contains(err.Error(), "admin:secret") {
		t.Fatalf("URL credential 未遮蔽：%v", err)
	}
}
