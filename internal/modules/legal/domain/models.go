package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type DocumentStatus string

const (
	StatusDraft     DocumentStatus = "draft"
	StatusScheduled DocumentStatus = "scheduled"
	StatusPublished DocumentStatus = "published"
	StatusArchived  DocumentStatus = "archived"
)

type UpdateClassification string

const (
	ClassificationMaterial UpdateClassification = "material"
	ClassificationMinor    UpdateClassification = "minor"
	ClassificationUrgent   UpdateClassification = "urgent"
)

type TableData struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

type Section struct {
	ID      string     `json:"id"`
	Order   int        `json:"order"`
	Title   string     `json:"title"`
	Content string     `json:"content"`
	Summary string     `json:"summary,omitempty"`
	Table   *TableData `json:"table,omitempty"`
}

type Document struct {
	Key       string    `json:"key" db:"key"`
	Audience  string    `json:"audience" db:"audience"`
	Locale    string    `json:"locale" db:"locale"`
	Title     string    `json:"title" db:"title"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type DocumentVersion struct {
	ID                   uuid.UUID            `json:"id" db:"id"`
	DocumentKey          string               `json:"document_key" db:"document_key"`
	DisplayVersion       string               `json:"display_version" db:"display_version"`
	SchemaVersion        int                  `json:"schema_version" db:"schema_version"`
	Title                string               `json:"title" db:"title"`
	Summary              string               `json:"summary" db:"summary"`
	Sections             []Section            `json:"sections"`
	SectionsRaw          []byte               `json:"-" db:"sections"`
	ContentHash          string               `json:"content_hash" db:"content_hash"`
	UpdateClassification UpdateClassification `json:"update_classification" db:"update_classification"`
	Status               DocumentStatus       `json:"status" db:"status"`
	PublishedAt          time.Time            `json:"published_at" db:"published_at"`
	EffectiveAt          time.Time            `json:"effective_at" db:"effective_at"`
	SupersededAt         *time.Time           `json:"superseded_at,omitempty" db:"superseded_at"`
	CreatedBy            *uuid.UUID           `json:"created_by,omitempty" db:"created_by"`
	CreatedAt            time.Time            `json:"created_at" db:"created_at"`
}

func ComputeContentHash(title, summary string, sections []Section) string {
	payload, _ := json.Marshal(struct {
		Title    string    `json:"title"`
		Summary  string    `json:"summary"`
		Sections []Section `json:"sections"`
	}{
		Title:    title,
		Summary:  summary,
		Sections: sections,
	})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

type Acceptance struct {
	ID          uuid.UUID `json:"id" db:"id"`
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	DocumentKey string    `json:"document_key" db:"document_key"`
	VersionID   uuid.UUID `json:"version_id" db:"version_id"`
	ContentHash string    `json:"content_hash" db:"content_hash"`
	Action      string    `json:"action" db:"action"`
	Context     string    `json:"context" db:"context"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type PublicationAudit struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	DocumentKey string     `json:"document_key" db:"document_key"`
	VersionID   uuid.UUID  `json:"version_id" db:"version_id"`
	Action      string     `json:"action" db:"action"`
	ActorID     *uuid.UUID `json:"actor_id,omitempty" db:"actor_id"`
	Notes       string     `json:"notes" db:"notes"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

type Requirement struct {
	DocumentKey    string           `json:"document_key"`
	Title          string           `json:"title"`
	CurrentVersion DocumentVersion  `json:"current_version"`
	Accepted       bool             `json:"accepted"`
	LastAcceptedAt *time.Time       `json:"last_accepted_at,omitempty"`
}
