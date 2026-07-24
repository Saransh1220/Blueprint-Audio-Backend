package application

import (
	"math"
	"strings"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/require"
)

func validBeatForValidation() domain.Spec {
	return domain.Spec{
		Title:       "Valid beat",
		Category:    domain.CategoryBeat,
		BasePrice:   10,
		BPM:         120,
		Key:         "C MAJOR",
		Tags:        []string{"tag"},
		Moods:       []string{"Dark"},
		Instruments: []string{"Piano"},
		Genres:      []domain.Genre{{Name: "TRAP"}},
		Licenses: []domain.LicenseOption{{
			LicenseType: domain.LicenseBasic,
			Name:        "Basic",
			Price:       10,
		}},
	}
}

func TestValidateSpecCoversEveryValidationRule(t *testing.T) {
	tests := []struct {
		name string
		edit func(*domain.Spec)
		want string
	}{
		{"title too long", func(s *domain.Spec) { s.Title = strings.Repeat("x", 101) }, "title must be at most 100 characters"},
		{"description too long", func(s *domain.Spec) { s.Description = strings.Repeat("x", 501) }, "description must be at most 500 characters"},
		{"nan price", func(s *domain.Spec) { s.BasePrice = math.NaN() }, "price cannot be negative"},
		{"infinite price", func(s *domain.Spec) { s.BasePrice = math.Inf(1) }, "price cannot be negative"},
		{"invalid category", func(s *domain.Spec) { s.Category = domain.Category("other") }, "invalid category"},
		{"too many tags", func(s *domain.Spec) { s.Tags = []string{"a", "b", "c", "d"} }, "maximum 3 tags allowed"},
		{"empty tag", func(s *domain.Spec) { s.Tags = []string{" "} }, "invalid tags"},
		{"long tag", func(s *domain.Spec) { s.Tags = []string{strings.Repeat("x", 31)} }, "invalid tags"},
		{"duplicate tags", func(s *domain.Spec) { s.Tags = []string{"Tag", "tag"} }, "duplicate tags"},
		{"invalid mood", func(s *domain.Spec) { s.Moods = []string{"Happy"} }, "invalid moods"},
		{"invalid instrument", func(s *domain.Spec) { s.Instruments = []string{"Flute"} }, "invalid instruments"},
		{"low bpm", func(s *domain.Spec) { s.BPM = 59 }, "BPM must be between 60 and 300"},
		{"high bpm", func(s *domain.Spec) { s.BPM = 301 }, "BPM must be between 60 and 300"},
		{"invalid key", func(s *domain.Spec) { s.Key = "H MAJOR" }, "invalid musical key"},
		{"missing genre", func(s *domain.Spec) { s.Genres = nil }, "exactly one genre is required"},
		{"too many genres", func(s *domain.Spec) { s.Genres = []domain.Genre{{Name: "TRAP"}, {Name: "POP"}} }, "exactly one genre is required"},
		{"invalid genre", func(s *domain.Spec) { s.Genres = []domain.Genre{{Name: "POLKA"}} }, "invalid genre"},
		{"missing licenses", func(s *domain.Spec) { s.Licenses = nil }, "at least one license is required"},
		{"invalid license type", func(s *domain.Spec) { s.Licenses[0].LicenseType = domain.LicenseType("Other") }, "invalid license type"},
		{"duplicate license", func(s *domain.Spec) { s.Licenses = append(s.Licenses, s.Licenses[0]) }, "duplicate license type"},
		{"empty license name", func(s *domain.Spec) { s.Licenses[0].Name = " " }, "license name must be between 1 and 100 characters"},
		{"long license name", func(s *domain.Spec) { s.Licenses[0].Name = strings.Repeat("x", 101) }, "license name must be between 1 and 100 characters"},
		{"nan license price", func(s *domain.Spec) { s.Licenses[0].Price = math.NaN() }, "license price cannot be negative"},
		{"infinite license price", func(s *domain.Spec) { s.Licenses[0].Price = math.Inf(-1) }, "license price cannot be negative"},
		{"too many license features", func(s *domain.Spec) {
			s.Licenses[0].Features = make([]string, maxLicenseFeatures+1)
			for i := range s.Licenses[0].Features {
				s.Licenses[0].Features[i] = string(rune('a' + i))
			}
		}, "maximum 20 license features allowed"},
		{"empty license feature", func(s *domain.Spec) { s.Licenses[0].Features = []string{" "} }, "invalid license features"},
		{"long license feature", func(s *domain.Spec) {
			s.Licenses[0].Features = []string{strings.Repeat("x", maxLicenseFeatureLength+1)}
		}, "invalid license features"},
		{"duplicate license features", func(s *domain.Spec) { s.Licenses[0].Features = []string{"Untagged", "untagged"} }, "duplicate license features"},
		{"too many license file types", func(s *domain.Spec) {
			s.Licenses[0].FileTypes = make([]string, maxLicenseFileTypes+1)
			for i := range s.Licenses[0].FileTypes {
				s.Licenses[0].FileTypes[i] = string(rune('a' + i))
			}
		}, "maximum 10 license file types allowed"},
		{"empty license file type", func(s *domain.Spec) { s.Licenses[0].FileTypes = []string{" "} }, "invalid license file types"},
		{"long license file type", func(s *domain.Spec) {
			s.Licenses[0].FileTypes = []string{strings.Repeat("x", maxLicenseFileTypeLength+1)}
		}, "invalid license file types"},
		{"duplicate license file types", func(s *domain.Spec) { s.Licenses[0].FileTypes = []string{"WAV", "wav"} }, "duplicate license file types"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := validBeatForValidation()
			tt.edit(&spec)
			require.EqualError(t, validateSpec(&spec), tt.want)
		})
	}

	require.NoError(t, validateSpec(func() *domain.Spec { s := validBeatForValidation(); return &s }()))
	require.NoError(t, validateSpec(&domain.Spec{Title: "Valid sample", Category: domain.CategorySample}))
}

func TestValidateSpecNormalizesNotNullDatabaseArrays(t *testing.T) {
	spec := validBeatForValidation()
	spec.Moods = nil
	spec.Instruments = nil
	spec.Licenses[0].Features = nil
	spec.Licenses[0].FileTypes = nil

	require.NoError(t, validateSpec(&spec))
	require.NotNil(t, spec.Moods)
	require.Empty(t, spec.Moods)
	require.NotNil(t, spec.Instruments)
	require.Empty(t, spec.Instruments)
	require.NotNil(t, spec.Licenses[0].Features)
	require.Empty(t, spec.Licenses[0].Features)
	require.NotNil(t, spec.Licenses[0].FileTypes)
	require.Empty(t, spec.Licenses[0].FileTypes)
}
