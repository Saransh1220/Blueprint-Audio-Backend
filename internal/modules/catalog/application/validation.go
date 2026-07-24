package application

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

var allowedKeys = set("C MAJOR", "C# MAJOR", "D MAJOR", "D# MAJOR", "E MAJOR", "F MAJOR", "F# MAJOR", "G MAJOR", "G# MAJOR", "A MAJOR", "A# MAJOR", "B MAJOR", "C MINOR", "C# MINOR", "D MINOR", "D# MINOR", "E MINOR", "F MINOR", "F# MINOR", "G MINOR", "G# MINOR", "A MINOR", "A# MINOR", "B MINOR")
var allowedGenres = set("TRAP", "DRILL", "R&B", "EXPERIMENTAL", "HOUSE", "LO-FI", "HIP-HOP", "POP", "TECH", "AMBIENT")
var allowedMoods = set("Moody", "Dark", "Cinematic", "Dreamy", "Aggressive", "Soulful", "Melancholic")
var allowedInstruments = set("Piano", "Guitar", "Drums", "Synth", "Bass", "Strings", "Brass", "808", "Vocal")

const (
	maxLicenseFeatures       = 20
	maxLicenseFeatureLength  = 200
	maxLicenseFileTypes      = 10
	maxLicenseFileTypeLength = 50
)

func set(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validateSpec(spec *domain.Spec) error {
	normalizeDatabaseArrays(spec)

	titleLength := utf8.RuneCountInString(strings.TrimSpace(spec.Title))
	if titleLength == 0 {
		return fmt.Errorf("title is required")
	}
	if titleLength > 100 {
		return fmt.Errorf("title must be at most 100 characters")
	}
	if utf8.RuneCountInString(spec.Description) > 500 {
		return fmt.Errorf("description must be at most 500 characters")
	}
	if math.IsNaN(spec.BasePrice) || math.IsInf(spec.BasePrice, 0) || spec.BasePrice < 0 {
		return fmt.Errorf("price cannot be negative")
	}
	if spec.Category != domain.CategoryBeat && spec.Category != domain.CategorySample {
		return fmt.Errorf("invalid category")
	}
	if err := validateStringList("tags", spec.Tags, 3, 30, nil); err != nil {
		return err
	}
	if err := validateStringList("moods", spec.Moods, 5, 30, allowedMoods); err != nil {
		return err
	}
	if err := validateStringList("instruments", spec.Instruments, 5, 30, allowedInstruments); err != nil {
		return err
	}
	if spec.Category == domain.CategorySample {
		return nil
	}
	if spec.BPM < 60 || spec.BPM > 300 {
		return fmt.Errorf("BPM must be between 60 and 300")
	}
	if _, ok := allowedKeys[strings.ToUpper(strings.TrimSpace(spec.Key))]; !ok {
		return fmt.Errorf("invalid musical key")
	}
	if len(spec.Genres) != 1 {
		return fmt.Errorf("exactly one genre is required")
	}
	for _, genre := range spec.Genres {
		if _, ok := allowedGenres[strings.ToUpper(strings.TrimSpace(genre.Name))]; !ok {
			return fmt.Errorf("invalid genre")
		}
	}
	if len(spec.Licenses) == 0 {
		return fmt.Errorf("at least one license is required")
	}
	licenses := map[domain.LicenseType]struct{}{}
	for _, license := range spec.Licenses {
		if license.LicenseType != domain.LicenseBasic && license.LicenseType != domain.LicensePremium && license.LicenseType != domain.LicenseTrackout && license.LicenseType != domain.LicenseUnlimited {
			return fmt.Errorf("invalid license type")
		}
		if _, exists := licenses[license.LicenseType]; exists {
			return fmt.Errorf("duplicate license type")
		}
		licenses[license.LicenseType] = struct{}{}
		if n := utf8.RuneCountInString(strings.TrimSpace(license.Name)); n < 1 || n > 100 {
			return fmt.Errorf("license name must be between 1 and 100 characters")
		}
		if math.IsNaN(license.Price) || math.IsInf(license.Price, 0) || license.Price < 0 {
			return fmt.Errorf("license price cannot be negative")
		}
		if err := validateStringList(
			"license features",
			license.Features,
			maxLicenseFeatures,
			maxLicenseFeatureLength,
			nil,
		); err != nil {
			return err
		}
		if err := validateStringList(
			"license file types",
			license.FileTypes,
			maxLicenseFileTypes,
			maxLicenseFileTypeLength,
			nil,
		); err != nil {
			return err
		}
	}
	return nil
}

func normalizeDatabaseArrays(spec *domain.Spec) {
	if spec.Moods == nil {
		spec.Moods = []string{}
	}
	if spec.Instruments == nil {
		spec.Instruments = []string{}
	}
	for i := range spec.Licenses {
		if spec.Licenses[i].Features == nil {
			spec.Licenses[i].Features = []string{}
		}
		if spec.Licenses[i].FileTypes == nil {
			spec.Licenses[i].FileTypes = []string{}
		}
	}
}

func validateStringList(name string, values []string, max, valueMax int, allowed map[string]struct{}) error {
	if len(values) > max {
		return fmt.Errorf("maximum %d %s allowed", max, name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || utf8.RuneCountInString(value) > valueMax {
			return fmt.Errorf("invalid %s", name)
		}
		if allowed != nil {
			if _, ok := allowed[value]; !ok {
				return fmt.Errorf("invalid %s", name)
			}
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate %s", name)
		}
		seen[key] = struct{}{}
	}
	return nil
}
