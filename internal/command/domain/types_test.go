package domain

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{value: "10m", want: 10 * time.Minute},
		{value: "2h", want: 2 * time.Hour},
		{value: "7d", want: 7 * 24 * time.Hour},
		{value: "8d", wantErr: true},
		{value: "0m", wantErr: true},
		{value: "10s", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseDuration(tt.value)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("ParseDuration() = %v, %v，預期 %v，錯誤=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestParseReason(t *testing.T) {
	t.Parallel()
	if _, err := ParseReason(strings.Repeat("警", 201)); err == nil {
		t.Fatal("超長原因應被拒絕")
	}
	if got, err := ParseReason("  測試原因  "); err != nil || got != "測試原因" {
		t.Fatalf("ParseReason() = %q, %v", got, err)
	}
}
