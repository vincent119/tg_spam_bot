package application

import "time"

// RepeatAction 定義獨立於內容規則與 AI 的精確重複處置政策。
type RepeatAction string

const (
	// RepeatActionObserve 為保守預設，僅記錄重複訊號。
	RepeatActionObserve RepeatAction = "observe"
	// RepeatActionDelete 清除快照內可追蹤的重複訊息。
	RepeatActionDelete RepeatAction = "delete"
	// RepeatActionBan 在刪文後依全域模式封鎖同群成員。
	RepeatActionBan RepeatAction = "ban"
)

// RepeatPlan 是同一觀測快照與全域模式得出的處置意圖。
type RepeatPlan struct {
	Triggered  bool
	Count      int
	Window     time.Duration
	Action     RepeatAction
	Mode       Mode
	MessageIDs []int64
	Ban        bool
	Truncated  bool
}

// PlanRepeat 只使用可信的原子快照；容量降級或缺少目前訊息時不建立破壞性計畫。
func PlanRepeat(snapshot RepeatSnapshot, threshold int, action RepeatAction, mode Mode) RepeatPlan {
	if !snapshot.Available || !snapshot.CurrentObserved || threshold < 2 || snapshot.Count < threshold {
		return RepeatPlan{}
	}
	plan := RepeatPlan{Triggered: true, Count: snapshot.Count, Window: snapshot.Window, Action: action, Mode: mode, Truncated: snapshot.Truncated}
	if action == RepeatActionObserve || mode == ModeObserve {
		return plan
	}
	if action != RepeatActionDelete && action != RepeatActionBan {
		return plan
	}
	plan.MessageIDs = append([]int64(nil), snapshot.MessageIDs...)
	plan.Ban = action == RepeatActionBan && mode == ModeEnforce
	return plan
}
