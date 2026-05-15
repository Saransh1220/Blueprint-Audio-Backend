package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

const (
	defaultSpecPage  = 1
	defaultSpecLimit = 20
	maxSpecLimit     = 50
	shortCodeLength  = 8
)

type SpecService interface {
	CreateSpec(ctx context.Context, spec *domain.Spec) error
	GetSpec(ctx context.Context, id uuid.UUID) (*domain.Spec, error)
	ListSpecs(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error)
	UpdateSpec(ctx context.Context, spec *domain.Spec, producerID uuid.UUID) error
	UpdateFilesAndStatus(ctx context.Context, id uuid.UUID, files map[string]*string, status domain.ProcessingStatus) error
	DeleteSpec(ctx context.Context, id uuid.UUID, producerId uuid.UUID) error
	GetUserSpecs(ctx context.Context, producerID uuid.UUID, page, limit int) ([]domain.Spec, int, error)
	GetSpecByShortCode(ctx context.Context, code string) (*domain.Spec, error)
	GetSpecBySlug(ctx context.Context, slug string) (*domain.Spec, error)
}

type specService struct {
	repo domain.SpecRepository
}

func NewSpecService(repo domain.SpecRepository) SpecService {
	return &specService{repo: repo}
}

func (s *specService) CreateSpec(ctx context.Context, spec *domain.Spec) error {
	if spec.Title == "" {
		return errors.New("title is required")
	}
	if spec.BasePrice < 0 {
		return errors.New("price cannot be negative")
	}
	if spec.Category == domain.CategoryBeat {
		if spec.BPM < 50 || spec.BPM > 300 {
			return errors.New("BPM must be between 50 and 300")
		}

		if spec.ProcessingStatus != domain.ProcessingStatusProcessing {
			if spec.WavUrl == nil || *spec.WavUrl == "" {
				return errors.New("WAV file is required!")
			}
			if spec.StemsUrl == nil || *spec.StemsUrl == "" {
				return errors.New("stems file is mandatory for beats")
			}
		}
	}

	if len(spec.Moods) > 5 {
		return errors.New("maximum 5 moods allowed")
	}
	if len(spec.Instruments) > 5 {
		return errors.New("maximum 5 instruments allowed")
	}

	if spec.Slug == nil || strings.TrimSpace(*spec.Slug) == "" {
		slug, err := s.generateUniqueSlug(ctx, spec.Title)
		if err != nil {
			return err
		}
		spec.Slug = &slug
	}

	if spec.ShortCode == nil || strings.TrimSpace(*spec.ShortCode) == "" {
		shortCode, err := s.generateUniqueShortCode(ctx)
		if err != nil {
			return err
		}
		spec.ShortCode = &shortCode
	}

	return s.repo.Create(ctx, spec)
}

func (s *specService) GetSpec(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	return s.repo.GetByID(ctx, id)
}

// The `GetSpecByShortCode` function in the `specService` struct is used to retrieve a specific spec by
// its short code from the repository. It takes the context and the short code string as input
// parameters and returns a pointer to the `domain.Spec` struct along with an error. Inside the
// function, it calls the `GetByShortCode` method of the repository (`s.repo`) passing the context and
// the short code to fetch the spec based on the provided short code.
func (s *specService) GetSpecByShortCode(ctx context.Context, code string) (*domain.Spec, error) {
	return s.repo.GetByShortCode(ctx, code)
}

// The `GetSpecBySlug` function in the `specService` struct is used to retrieve a specific spec by its
// slug from the repository. It takes the context and the slug string as input parameters and returns a
// pointer to the `domain.Spec` struct along with an error. Inside the function, it calls the
// `GetBySlug` method of the repository (`s.repo`) passing the context and the slug to fetch the spec
// based on the provided slug.
func (s *specService) GetSpecBySlug(ctx context.Context, slug string) (*domain.Spec, error) {
	return s.repo.GetBySlug(ctx, slug)
}

func (s *specService) ListSpecs(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
	filter.Page, filter.Limit = normalizePageAndLimit(filter.Page, filter.Limit)
	filter.Offset = (filter.Page - 1) * filter.Limit
	return s.repo.List(ctx, filter)
}

func (s *specService) DeleteSpec(ctx context.Context, id uuid.UUID, producerId uuid.UUID) error {
	return s.repo.Delete(ctx, id, producerId)
}

// UpdateSpec updates a spec's metadata with ownership validation
func (s *specService) UpdateSpec(ctx context.Context, spec *domain.Spec, producerID uuid.UUID) error {
	// Validate ownership
	existing, err := s.repo.GetByID(ctx, spec.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("spec not found")
	}
	if existing.ProducerID != producerID {
		return errors.New("unauthorized: you can only update your own specs")
	}

	// Validate updates
	if spec.Title == "" {
		return errors.New("title is required")
	}
	if spec.BasePrice < 0 {
		return errors.New("price cannot be negative")
	}
	if spec.Category == domain.CategoryBeat {
		if spec.BPM < 50 || spec.BPM > 300 {
			return errors.New("BPM must be between 50 and 300")
		}
	}

	// Set producer ID to ensure it doesn't change
	spec.ProducerID = producerID
	spec.ShortCode = existing.ShortCode
	if spec.Slug == nil || strings.TrimSpace(*spec.Slug) == "" {
		spec.Slug = existing.Slug
	}
	return s.repo.Update(ctx, spec)
}

// GetUserSpecs retrieves all specs for a specific producer with pagination
func (s *specService) GetUserSpecs(ctx context.Context, producerID uuid.UUID, page, limit int) ([]domain.Spec, int, error) {
	page, limit = normalizePageAndLimit(page, limit)
	offset := (page - 1) * limit
	return s.repo.ListByUserID(ctx, producerID, limit, offset)
}

func (s *specService) UpdateFilesAndStatus(ctx context.Context, id uuid.UUID, files map[string]*string, status domain.ProcessingStatus) error {
	return s.repo.UpdateFilesAndStatus(ctx, id, files, status)
}

func normalizePageAndLimit(page, limit int) (int, int) {
	if page < 1 {
		page = defaultSpecPage
	}
	if limit <= 0 {
		limit = defaultSpecLimit
	}
	if limit > maxSpecLimit {
		limit = maxSpecLimit
	}
	return page, limit
}

func (s *specService) generateUniqueSlug(ctx context.Context, title string) (string, error) {
	base := slugify(title)
	if base == "" {
		base = "untitled-beat"
	}

	slug := base
	for attempt := 0; attempt < 10; attempt++ {
		existing, err := s.repo.GetBySlug(ctx, slug)
		if err == nil && existing != nil {
			slug = fmt.Sprintf("%s-%s", base, randomCodeSegment(3))
			continue
		}
		return slug, nil
	}

	return "", errors.New("failed to generate unique slug")
}

func (s *specService) generateUniqueShortCode(ctx context.Context) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		code := randomCodeSegment(shortCodeLength)
		existing, err := s.repo.GetByShortCode(ctx, code)
		if err == nil && existing != nil {
			continue
		}
		return code, nil
	}

	return "", errors.New("failed to generate unique short code")
}

func slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var out []rune
	prevDash := false

	for _, r := range input {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			out = append(out, r)
			prevDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.':
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
		}
	}

	slug := strings.Trim(string(out), "-")
	return slug
}

func randomCodeSegment(length int) string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if length <= 0 {
		return ""
	}
	if length > len(id) {
		return id
	}
	return strings.ToLower(id[:length])
}
