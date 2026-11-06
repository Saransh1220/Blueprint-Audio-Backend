package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeContentHash_DeterministicAndChangesOnContentChange(t *testing.T) {
	sections := []Section{
		{
			ID:      "section-1",
			Order:   1,
			Title:   "First Section",
			Content: "Some terms text here.",
		},
	}

	hash1 := ComputeContentHash("Terms", "Summary text", sections)
	hash2 := ComputeContentHash("Terms", "Summary text", sections)
	assert.Equal(t, hash1, hash2, "identical content must generate identical hashes")
	assert.Len(t, hash1, 64, "sha256 hex string must be 64 characters")

	// Changing summary changes hash
	hashDiffSummary := ComputeContentHash("Terms", "Altered summary", sections)
	assert.NotEqual(t, hash1, hashDiffSummary)

	// Changing section content changes hash
	sectionsAltered := []Section{
		{
			ID:      "section-1",
			Order:   1,
			Title:   "First Section",
			Content: "Modified content here.",
		},
	}
	hashDiffContent := ComputeContentHash("Terms", "Summary text", sectionsAltered)
	assert.NotEqual(t, hash1, hashDiffContent)
}
