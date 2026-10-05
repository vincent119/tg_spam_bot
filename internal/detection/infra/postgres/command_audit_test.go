package postgres

import (
	"strings"
	"testing"

	commanddomain "github.com/vincent119/tg_spam_bot/internal/command/domain"
)

func TestSafeCommandArgumentSummary(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		command commanddomain.Command
		want    string
	}{
		{name: "正常標記原因不落稽核", command: commanddomain.Command{Name: commanddomain.NameHam, Args: "event:tg:42 私密原因"}, want: "parameters_present"},
		{name: "垃圾標記參數不落稽核", command: commanddomain.Command{Name: commanddomain.NameSpam, Args: "分類 ban"}, want: "parameters_present"},
		{name: "空參數", command: commanddomain.Command{Name: commanddomain.NameHam}, want: ""},
		{name: "舊指令維持摘要", command: commanddomain.Command{Name: commanddomain.NameWarnings, Args: "30d"}, want: "30d"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := safeCommandArgumentSummary(tt.command)
			if got != tt.want || strings.Contains(got, "私密原因") {
				t.Fatalf("稽核參數摘要=%q，預期=%q", got, tt.want)
			}
		})
	}
}
