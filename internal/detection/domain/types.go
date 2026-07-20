// Package domain 定義與框架無關的訊息、規則、正規化與判定模型。
package domain

import "time"

// ContentSource 標示規則命中的是原文正規化結果或繁體轉換副本。
type ContentSource string

// 支援的命中內容來源。
const (
	SourceNormalized           ContentSource = "normalized"
	SourceTraditional          ContentSource = "traditional_variant"
	SourceReferenceNormalized  ContentSource = "reference_normalized"
	SourceReferenceTraditional ContentSource = "reference_traditional_variant"
)

// Message 是與 Telegram 傳輸格式解耦的偵測輸入。
type Message struct {
	UpdateID  int64
	ChatID    int64
	MessageID int64
	UserID    int64
	Text      string
	// ReferenceText 與發送者輸入分離，避免將被引用內容的聯絡訊號誤算為發送者行為。
	ReferenceText string
	Entities      []Entity
	JoinedAt      time.Time
	ReceivedAt    time.Time
}

// Entity 保存 URL 與 mention 等 Telegram 結構化資訊。
type Entity struct {
	Type string
	URL  string
}

// NewMessage 複製 slice 邊界，避免呼叫端後續修改影響領域資料。
func NewMessage(m Message) Message {
	m.Entities = append([]Entity(nil), m.Entities...)
	return m
}

// EntitiesCopy 回傳獨立副本，避免領域內部狀態外洩。
func (m Message) EntitiesCopy() []Entity {
	return append([]Entity(nil), m.Entities...)
}

// NormalizedText 同時保留原文與兩條比對軌道，避免繁簡轉換覆寫語意。
type NormalizedText struct {
	Original           string
	Normalized         string
	TraditionalVariant string
}

// Severity 定義違規風險等級。
type Severity string

// 支援的違規嚴重度。
const (
	SeverityNormal   Severity = "normal"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Action 定義分類規則允許產生的處置策略。
type Action string

// 支援的規則處置策略。
const (
	ActionObserve     Action = "observe"
	ActionDelete      Action = "delete"
	ActionProgressive Action = "progressive"
	ActionBan         Action = "ban"
)

// Category 是一組可獨立評分及設定門檻的違規類型。
type Category struct {
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name"`
	Severity   Severity `yaml:"severity"`
	Action     Action   `yaml:"action"`
	Threshold  int      `yaml:"threshold"`
	Weight     int      `yaml:"weight"`
	Enabled    bool     `yaml:"enabled"`
	Terms      []string `yaml:"terms"`
	Aliases    []string `yaml:"aliases"`
	RequireAny []string `yaml:"require_any"`
}

// RuleSet 是啟動時完整驗證並固定版本的規則快照。
type RuleSet struct {
	Version    string     `yaml:"version"`
	Categories []Category `yaml:"categories"`
}

// Match 保存可稽核的單項命中來源與分數。
type Match struct {
	RuleID string
	Term   string
	Source ContentSource
	Weight int
}

// Result 是偵測器輸出的可解釋判定。
type Result struct {
	Spam        bool
	CategoryID  string
	Severity    Severity
	Action      Action
	Score       int
	Threshold   int
	RuleVersion string
	Matches     []Match
	Signals     []string
}

// MatchesCopy 回傳命中明細的獨立副本。
func (r Result) MatchesCopy() []Match { return append([]Match(nil), r.Matches...) }

// SignalsCopy 回傳行為訊號的獨立副本。
func (r Result) SignalsCopy() []string { return append([]string(nil), r.Signals...) }
