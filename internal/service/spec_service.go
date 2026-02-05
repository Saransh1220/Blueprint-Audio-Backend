package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type SpecService interface {
	CreateSpec(ctx context.Context, spec *domain.Spec) error
	GetSpec(ctx context.Context, id uuid.UUID) (*domain.Spec, error)
	ListSpecs(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error)
	UpdateSpec(ctx context.Context, spec *domain.Spec, producerID uuid.UUID) error
	DeleteSpec(ctx context.Context, id uuid.UUID, producerId uuid.UUID) error
	GetUserSpecs(ctx context.Context, producerID uuid.UUID, page int) ([]domain.Spec, int, error)
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
			return errors.New("BPM must be between 60 and 200")
		}

		if spec.WavUrl == nil || *spec.WavUrl == "" {
			return errors.New("WAV file is required!")
		}
		if spec.StemsUrl == nil || *spec.StemsUrl == "" {
			return errors.New("stems file is mandatory for beats")
		}
	}
	return s.repo.Create(ctx, spec)
}

func (s *specService) GetSpec(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *specService) ListSpecs(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
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
	return s.repo.Update(ctx, spec)
}

// GetUserSpecs retrieves all specs for a specific producer with pagination
func (s *specService) GetUserSpecs(ctx context.Context, producerID uuid.UUID, page int) ([]domain.Spec, int, error) {
	limit := 20
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByUserID(ctx, producerID, limit, offset)
}
