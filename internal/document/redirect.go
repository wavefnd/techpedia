package document

import "time"

// Redirect is an administrator-managed public document alias. TargetWID keeps
// the alias attached to the same document when its canonical name changes.
type Redirect struct {
	Locale       string
	Name         string
	Slug         string
	TargetWID    WID
	TargetName   string
	TargetSlug   string
	TargetExists bool
	CreatedBy    string
	CreatedAt    time.Time
}
