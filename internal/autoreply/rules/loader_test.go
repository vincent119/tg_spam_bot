package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantLen int
		wantErr bool
	}{
		{
			name: "valid",
			content: `
version: "2026-07-20.1"
rules:
  - id: download_page
    keywords: ["下載頁", "app 去哪裡下載"]
    reply: "下載頁在：https://example.com/download"
`,
			wantLen: 1,
		},
		{
			name: "disabled rule",
			content: `
rules:
  - id: download_page
    enabled: false
    keywords: ["下載頁"]
    reply: "下載頁在：https://example.com/download"
`,
			wantLen: 1,
		},
		{
			name: "duplicate id",
			content: `
rules:
  - id: download_page
    keywords: ["下載頁"]
    reply: "下載頁在：https://example.com/download"
  - id: download_page
    keywords: ["app"]
    reply: "app"
`,
			wantErr: true,
		},
		{
			name: "empty keyword",
			content: `
rules:
  - id: download_page
    keywords: [""]
    reply: "下載頁在：https://example.com/download"
`,
			wantErr: true,
		},
		{
			name: "empty reply",
			content: `
rules:
  - id: download_page
    keywords: ["下載頁"]
    reply: ""
`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "auto_replies.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("建立測試規則檔失敗：%v", err)
			}
			got, err := LoadFile(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadFile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && len(got.Rules) != tt.wantLen {
				t.Fatalf("規則數 = %d，預期 %d", len(got.Rules), tt.wantLen)
			}
		})
	}
}
