package domain

import "time"

type ContentSource string

const (
	SourceNormalized  ContentSource = "normalized"
	SourceTraditional ContentSource = "traditional_variant"
)

type Message struct {
	UpdateID   int64
	ChatID     int64
	MessageID  int64
	UserID     int64
	Text       string
	Entities   []Entity
	JoinedAt   time.Time
	ReceivedAt time.Time
}

type Entity struct {
	Type string
	URL  string
}

func NewMessage(m Message) Message {
	m.Entities = append([]Entity(nil), m.Entities...)
	return m
}

func (m Message) EntitiesCopy() []Entity {
	return append([]Entity(nil), m.Entities...)
}

type NormalizedText struct {
	Original           string
	Normalized         string
	TraditionalVariant string
}

type Severity string

const (
	SeverityNormal   Severity = "normal"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Action string

const (
	ActionObserve     Action = "observe"
	ActionDelete      Action = "delete"
	ActionProgressive Action = "progressive"
	ActionBan         Action = "ban"
)

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

type RuleSet struct {
	Version    string     `yaml:"version"`
	Categories []Category `yaml:"categories"`
}

type Match struct {
	RuleID string
	Term   string
	Source ContentSource
	Weight int
}

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

func (r Result) MatchesCopy() []Match  { return append([]Match(nil), r.Matches...) }
func (r Result) SignalsCopy() []string { return append([]string(nil), r.Signals...) }
