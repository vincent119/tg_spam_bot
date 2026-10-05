package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// PreviewConfig 固定目前部署的政策；雜湊金鑰不會出現在預覽結果中。
type PreviewConfig struct {
	Mode            Mode
	RepeatAction    RepeatAction
	RepeatThreshold int
	HashKey         []byte
	AIEnabled       bool
}

// HistoricalDetection 保存當時已記錄的判定摘要，與目前規則重新計算結果分開。
type HistoricalDetection struct {
	RuleVersion string
	CategoryID  string
	Score       int
	Threshold   int
	Spam        bool
	Mode        Mode
	CreatedAt   time.Time
}

// HistoricalDetectionReader 僅查詢本群指定訊息已保存的偵測摘要。
type HistoricalDetectionReader interface {
	FindHistoricalDetection(ctx context.Context, chatID, messageID int64) (HistoricalDetection, bool, error)
}

// PreviewHistory 明確區分已記錄、未觀測及查詢不可得。
type PreviewHistory struct {
	Status string
	Record HistoricalDetection
}

// PreviewRepeat 描述目前窗口與假設納入目標後的唯一訊息數。
type PreviewRepeat struct {
	Available       bool
	ExistingCount   int
	ProjectedCount  int
	Threshold       int
	Window          time.Duration
	ObservedAt      time.Time
	CurrentObserved bool
	Truncated       bool
	CandidateCount  int
	WouldTrigger    bool
}

// PreviewResult 是目前規則重新計算的診斷摘要，不代表當時已執行的處置。
type PreviewResult struct {
	Rule            domain.Result
	Repeat          PreviewRepeat
	Historical      PreviewHistory
	ExemptionKnown  bool
	Exempt          bool
	ExemptionReason string
	AIStatus        string
	AIReason        string
	Mode            Mode
	RepeatAction    RepeatAction
	PlannedActions  []ActionKind
	ActionKnown     bool
	ActionReason    string
}

// PreviewService 只持有判定器與唯讀查詢，不接觸正式觀測、違規及 Telegram 處置端口。
type PreviewService struct {
	detector   Detector
	repeats    RepeatSnapshotReader
	exemptions ExemptionStore
	history    HistoricalDetectionReader
	config     PreviewConfig
}

// PreviewOption 加入非必要的唯讀歷史查詢。
type PreviewOption func(*PreviewService)

// WithPreviewHistory 提供已保存偵測摘要的唯讀查詢。
func WithPreviewHistory(reader HistoricalDetectionReader) PreviewOption {
	return func(service *PreviewService) { service.history = reader }
}

// NewPreviewService 建立不具正式寫入能力的判定預覽服務。
func NewPreviewService(detector Detector, repeats RepeatSnapshotReader, exemptions ExemptionStore, config PreviewConfig, opts ...PreviewOption) (*PreviewService, error) {
	if detector == nil || repeats == nil || exemptions == nil {
		return nil, errors.New("預覽需要判定器、重複查詢與豁免查詢")
	}
	if config.Mode != ModeObserve && config.Mode != ModeDeleteOnly && config.Mode != ModeEnforce {
		return nil, errors.New("預覽模式無效")
	}
	if config.RepeatAction != RepeatActionObserve && config.RepeatAction != RepeatActionDelete && config.RepeatAction != RepeatActionBan {
		return nil, errors.New("預覽重複政策無效")
	}
	if config.RepeatThreshold < 2 || len(config.HashKey) == 0 {
		return nil, errors.New("預覽重複門檻或雜湊金鑰未設定")
	}
	config.HashKey = append([]byte(nil), config.HashKey...)
	service := &PreviewService{detector: detector, repeats: repeats, exemptions: exemptions, config: config}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

// Preview 讀取當前狀態並重新計算；任何查詢失敗都保留「未知」而非假裝零次。
func (s *PreviewService) Preview(ctx context.Context, message domain.Message) (PreviewResult, error) {
	if message.ChatID == 0 || message.UserID == 0 || message.MessageID <= 0 || strings.TrimSpace(message.Text) == "" {
		return PreviewResult{}, errors.New("預覽目標缺少群組、使用者、訊息或文字")
	}
	result := PreviewResult{Mode: s.config.Mode, RepeatAction: s.config.RepeatAction, Historical: PreviewHistory{Status: "unavailable"}}
	if s.config.AIEnabled {
		result.AIStatus = "not_run"
		result.AIReason = "preview_does_not_call_ai"
	} else {
		result.AIStatus = "disabled"
		result.AIReason = "ai_disabled"
	}
	if exempt, reason, err := s.exemptions.IsExempt(ctx, message.ChatID, message.UserID); err == nil {
		result.ExemptionKnown = true
		result.Exempt = exempt
		result.ExemptionReason = reason
	}

	fingerprint := previewFingerprint(s.config.HashKey, message.Text)
	if snapshot, err := s.repeats.PeekRepeat(ctx, message, fingerprint); err == nil && snapshot.Available {
		result.Repeat = previewRepeat(snapshot, s.config.RepeatThreshold)
	} else {
		result.Repeat.Threshold = s.config.RepeatThreshold
	}
	if s.history != nil {
		if record, found, err := s.history.FindHistoricalDetection(ctx, message.ChatID, message.MessageID); err == nil {
			result.Historical.Status = "not_observed"
			if found {
				result.Historical = PreviewHistory{Status: "observed", Record: record}
			}
		}
	}

	if result.Repeat.Available && result.Repeat.WouldTrigger && !result.Exempt {
		result.Rule = s.detector.Detect(message, domain.SignalRepeatedContent)
	} else {
		result.Rule = s.detector.Detect(message)
	}
	result.PlannedActions, result.ActionKnown, result.ActionReason = s.previewActions(result)
	return result, nil
}

func previewRepeat(snapshot RepeatSnapshot, threshold int) PreviewRepeat {
	projected := snapshot.Count
	if !snapshot.CurrentObserved {
		projected++
	}
	return PreviewRepeat{
		Available: true, ExistingCount: snapshot.Count, ProjectedCount: projected,
		Threshold: threshold, Window: snapshot.Window, ObservedAt: snapshot.ObservedAt,
		CurrentObserved: snapshot.CurrentObserved, Truncated: snapshot.Truncated,
		CandidateCount: len(snapshot.MessageIDs), WouldTrigger: projected >= threshold,
	}
}

func (s *PreviewService) previewActions(result PreviewResult) ([]ActionKind, bool, string) {
	if !result.ExemptionKnown {
		return nil, false, "exemption_unknown"
	}
	if result.Exempt {
		return nil, true, "exempt"
	}
	if result.Mode == ModeObserve {
		return nil, true, "observe_mode"
	}
	var actions []ActionKind
	if result.Rule.Spam {
		switch {
		case result.Mode == ModeDeleteOnly:
			actions = append(actions, ActionDelete)
		case result.Rule.Severity == domain.SeverityCritical && result.Rule.Action == domain.ActionBan:
			actions = append(actions, ActionDelete, ActionBan)
		default:
			return nil, false, "violation_count_unknown"
		}
	}
	if !result.Repeat.Available && result.RepeatAction != RepeatActionObserve {
		return actions, false, "repeat_count_unknown"
	}
	if result.Repeat.Available && result.Repeat.WouldTrigger {
		switch result.RepeatAction {
		case RepeatActionDelete:
			actions = appendUniqueAction(actions, ActionDelete)
		case RepeatActionBan:
			actions = appendUniqueAction(actions, ActionDelete)
			if result.Mode == ModeEnforce {
				actions = appendUniqueAction(actions, ActionBan)
			}
		}
	}
	if s.config.AIEnabled && !result.Rule.Spam {
		decision := (AITriggerPolicy{}).Evaluate(result.Rule, AIEligibility{ChatAuthorized: true, MessageSupported: true})
		if decision.ShouldClassify {
			return actions, false, "ai_not_run"
		}
	}
	if !result.Repeat.CurrentObserved && result.Repeat.Available && result.Repeat.WouldTrigger {
		return actions, true, "hypothetical_unobserved"
	}
	return actions, true, "current_policy"
}

func appendUniqueAction(actions []ActionKind, kind ActionKind) []ActionKind {
	for _, action := range actions {
		if action == kind {
			return actions
		}
	}
	return append(actions, kind)
}

func previewFingerprint(key []byte, text string) string {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}
