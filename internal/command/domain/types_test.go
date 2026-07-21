package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewCommandCopiesBoundaries(t *testing.T) {
	t.Parallel()

	target := Target{ID: 2, Username: "target"}
	command, err := NewCommand(Command{
		UpdateID: 1, ChatID: -1001, MessageID: 3, Actor: Actor{ID: 1, Username: "actor"},
		Target: &target, Name: NameWarn, Args: "  理由  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	target.ID = 99
	if command.Target == nil || command.Target.ID != 2 || command.Args != "理由" {
		t.Fatalf("command=%+v", command)
	}
	if _, err := NewCommand(Command{}); err == nil {
		t.Fatal("缺少識別欄位應失敗")
	}
}

func TestDefinitionsAndLookup(t *testing.T) {
	t.Parallel()

	definitions := Definitions()
	if len(definitions) != 12 {
		t.Fatalf("指令數=%d，預期 12", len(definitions))
	}
	definitions[0].Usage = "changed"
	help, ok := LookupDefinition(NameHelp)
	if !ok || help.Usage != "/help" {
		t.Fatalf("help=%+v ok=%v", help, ok)
	}
	if _, ok := LookupDefinition(Name("unknown")); ok {
		t.Fatal("未知指令不應存在")
	}
}

func TestParseFeedSpamCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value   string
		want    string
		wantErr bool
	}{
		{value: "", want: "uncategorized_spam"},
		{value: " Agent_Recruiting ", want: "agent_recruiting"},
		{value: "crypto-exchange", want: "crypto-exchange"},
		{value: "中文", wantErr: true},
		{value: strings.Repeat("a", 65), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseFeedSpamCategory(tt.value)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("ParseFeedSpamCategory()=%q, %v，預期 %q err=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

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
			if (err != nil) != tt.wantErr || got.TimeDuration() != tt.want {
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

func TestParseUserIDAndStableError(t *testing.T) {
	t.Parallel()

	if id, err := ParseUserID(" 123 "); err != nil || id != 123 {
		t.Fatalf("ParseUserID()=%d, %v", id, err)
	}
	for _, value := range []string{"", "0", "-1", "abc"} {
		_, err := ParseUserID(value)
		var commandErr *CommandError
		if !errors.As(err, &commandErr) || commandErr.Code != ErrorInvalidInput || commandErr.Error() == "" {
			t.Fatalf("value=%q err=%v", value, err)
		}
	}
}
