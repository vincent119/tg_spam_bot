package application

import (
	"context"
	"errors"
	"time"

	"github.com/vincent119/tg_spam_bot/internal/detection/domain"
)

// ErrStaleManualFeedback 表示較新的人工修正已取代這次向量工作。
var ErrStaleManualFeedback = errors.New("manual feedback revision is stale")

const (
	// ManualFeedbackEmbeddingPending 表示向量建立工作尚未完成。
	ManualFeedbackEmbeddingPending = "pending_embedding"
	// ManualFeedbackEmbeddingCompleted 表示向量已建立。
	ManualFeedbackEmbeddingCompleted = "embedding_completed"
	// ManualFeedbackEmbeddingFailed 表示向量建立失敗但標記已保存。
	ManualFeedbackEmbeddingFailed = "embedding_failed"
	// ManualFeedbackEmbeddingUnavailable 表示目前未設定向量服務。
	ManualFeedbackEmbeddingUnavailable = "embedding_unavailable"
	// ManualFeedbackEmbeddingDisabled 表示此服務停用向量建立。
	ManualFeedbackEmbeddingDisabled = "embedding_disabled"
	// ManualFeedbackEmbeddingSuperseded 表示後續標記已取代本次向量工作。
	ManualFeedbackEmbeddingSuperseded = "embedding_superseded"
)

// ManualFeedback 保存單一群組、單一訊息目前有效的人工標記摘要。
type ManualFeedback struct {
	ID                 uint64
	ChatID             int64
	MessageID          int64
	TargetUserID       int64
	OperatorID         int64
	CommandUpdateID    int64
	ContentFingerprint string
	Label              domain.AILabel
	Category           string
	Reason             string
	Source             string
	Revision           uint64
	EmbeddingStatus    string
	EmbeddingErrorCode string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ManualFeedbackEvidence 是同群同指紋的有效人工證據；矛盾標籤不提供判定。
type ManualFeedbackEvidence struct {
	Present  bool
	Conflict bool
	Label    domain.AILabel
	Category string
}

// ScopedFeedbackEmbedding 僅對產生它的群組及仍有效的 revision 生效。
type ScopedFeedbackEmbedding struct {
	Feedback  ManualFeedback
	Embedding domain.EmbeddingResult
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ManualFeedbackStore 原子修訂人工標記並提供群組範圍的有效查詢。
type ManualFeedbackStore interface {
	SaveFeedback(ctx context.Context, feedback ManualFeedback) (ManualFeedback, bool, error)
	MarkFeedbackEmbeddingFailed(ctx context.Context, chatID, messageID int64, revision uint64, code string) error
	SaveScopedFeedbackEmbedding(ctx context.Context, embedding ScopedFeedbackEmbedding) error
	FindEffectiveFeedback(ctx context.Context, chatID int64, fingerprint string) (ManualFeedbackEvidence, error)
	SearchScopedManualSimilar(ctx context.Context, chatID int64, embedding domain.EmbeddingResult, maxNeighbors int) ([]domain.SemanticMatch, error)
	ManualFeedbackEpoch(ctx context.Context, chatID int64) (uint64, error)
}
