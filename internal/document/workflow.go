package document

import "time"

type ChangeAction string

const (
	ChangeCreate  ChangeAction = "create"
	ChangeEdit    ChangeAction = "edit"
	ChangeMove    ChangeAction = "move"
	ChangeRestore ChangeAction = "restore"
	ChangeDelete  ChangeAction = "delete"
)

type Change struct {
	ID        string       `json:"id"`
	WID       string       `json:"wid,omitempty"`
	Locale    string       `json:"locale"`
	Slug      string       `json:"slug"`
	Title     string       `json:"title"`
	Revision  string       `json:"revision,omitempty"`
	Author    string       `json:"author"`
	Summary   string       `json:"summary,omitempty"`
	Action    ChangeAction `json:"action"`
	CreatedAt time.Time    `json:"created_at"`
}

type ChangeFilter struct {
	Locale string
	Author string
	Query  string
	Action ChangeAction
	Since  time.Time
	Until  time.Time
	Limit  int
}

type Draft struct {
	UserID       string    `json:"user_id"`
	Locale       string    `json:"locale"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Source       string    `json:"source"`
	Summary      string    `json:"summary,omitempty"`
	BaseRevision string    `json:"base_revision,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TrashEntry struct {
	Document  Document  `json:"document"`
	DeletedBy string    `json:"deleted_by"`
	Reason    string    `json:"reason,omitempty"`
	DeletedAt time.Time `json:"deleted_at"`
}

type HistoryResetResult struct {
	Document           Document
	RemovedRevisions   int
	RemovedChanges     int
	RemovedDrafts      int
	RemovedSuggestions int
}

type AuditEntry struct {
	ID         string    `json:"id"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	Target     string    `json:"target"`
	Detail     string    `json:"detail,omitempty"`
	Result     string    `json:"result"`
	CreatedAt  time.Time `json:"created_at"`
}

type AuditFilter struct {
	Actor  string
	Action string
	Target string
	Limit  int
}
