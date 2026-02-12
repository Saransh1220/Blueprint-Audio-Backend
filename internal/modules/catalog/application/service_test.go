package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	createFn       func(context.Context, *domain.Spec) error
	getByIDFn      func(context.Context, uuid.UUID) (*domain.Spec, error)
	listFn         func(context.Context, domain.SpecFilter) ([]domain.Spec, int, error)
	updateFn       func(context.Context, *domain.Spec) error
	deleteFn       func(context.Context, uuid.UUID, uuid.UUID) error
	listByUserIDFn func(context.Context, uuid.UUID, int, int) ([]domain.Spec, int, error)
}

func (m mockRepo) Create(ctx context.Context, s *domain.Spec) error                          { return m.createFn(ctx, s) }
func (m mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Spec, error)           { return m.getByIDFn(ctx, id) }
func (m mockRepo) GetByIDSystem(context.Context, uuid.UUID) (*domain.Spec, error)            { return nil, nil }
func (m mockRepo) List(ctx context.Context, f domain.SpecFilter) ([]domain.Spec, int, error) { return m.listFn(ctx, f) }
func (m mockRepo) Update(ctx context.Context, s *domain.Spec) error                          { return m.updateFn(ctx, s) }
func (m mockRepo) Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error      { return m.deleteFn(ctx, id, producerID) }
func (m mockRepo) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.Spec, int, error) {
	return m.listByUserIDFn(ctx, producerID, limit, offset)
}

func TestSpecService_CreateSpecValidation(t *testing.T) {
	svc := NewSpecService(mockRepo{createFn: func(context.Context, *domain.Spec) error { return nil }})
	ctx := context.Background()

	err := svc.CreateSpec(ctx, &domain.Spec{})
	require.EqualError(t, err, "title is required")

	err = svc.CreateSpec(ctx, &domain.Spec{Title: "x", BasePrice: -1})
	require.EqualError(t, err, "price cannot be negative")

	err = svc.CreateSpec(ctx, &domain.Spec{Title: "x", BasePrice: 1, Category: domain.CategoryBeat, BPM: 20})
	require.EqualError(t, err, "BPM must be between 60 and 200")

	stems := "stems"
	err = svc.CreateSpec(ctx, &domain.Spec{Title: "x", BasePrice: 1, Category: domain.CategoryBeat, BPM: 120, StemsUrl: &stems})
	require.EqualError(t, err, "WAV file is required!")

	wav := "wav"
	err = svc.CreateSpec(ctx, &domain.Spec{Title: "x", BasePrice: 1, Category: domain.CategoryBeat, BPM: 120, WavUrl: &wav})
	require.EqualError(t, err, "stems file is mandatory for beats")

	err = svc.CreateSpec(ctx, &domain.Spec{Title: "ok", BasePrice: 1, Category: domain.CategorySample})
	require.NoError(t, err)
}

func TestSpecService_DelegatesAndUpdate(t *testing.T) {
	owner := uuid.New()
	specID := uuid.New()
	repo := mockRepo{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Spec, error) {
			return &domain.Spec{ID: specID, ProducerID: owner, Title: "old", Category: domain.CategoryBeat, BPM: 90}, nil
		},
		updateFn: func(_ context.Context, s *domain.Spec) error {
			if s.ProducerID != owner {
				return errors.New("bad owner")
			}
			return nil
		},
		listFn: func(context.Context, domain.SpecFilter) ([]domain.Spec, int, error) { return []domain.Spec{{ID: specID}}, 1, nil },
		deleteFn: func(context.Context, uuid.UUID, uuid.UUID) error { return nil },
		listByUserIDFn: func(_ context.Context, _ uuid.UUID, limit, offset int) ([]domain.Spec, int, error) {
			assert.Equal(t, 20, limit)
			assert.Equal(t, 0, offset)
			return []domain.Spec{{ID: specID}}, 1, nil
		},
	}
	svc := NewSpecService(repo)
	ctx := context.Background()

	_, _, err := svc.ListSpecs(ctx, domain.SpecFilter{})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteSpec(ctx, specID, owner))
	_, _, err = svc.GetUserSpecs(ctx, owner, -1)
	require.NoError(t, err)

	upd := &domain.Spec{ID: specID, Title: "new", BasePrice: 10, Category: domain.CategoryBeat, BPM: 100}
	require.NoError(t, svc.UpdateSpec(ctx, upd, owner))

	err = svc.UpdateSpec(ctx, &domain.Spec{ID: specID, Title: "", BasePrice: 10}, owner)
	require.EqualError(t, err, "title is required")

	err = svc.UpdateSpec(ctx, &domain.Spec{ID: specID, Title: "a", BasePrice: -1}, owner)
	require.EqualError(t, err, "price cannot be negative")

	err = svc.UpdateSpec(ctx, &domain.Spec{ID: specID, Title: "a", BasePrice: 1, Category: domain.CategoryBeat, BPM: 400}, owner)
	require.EqualError(t, err, "BPM must be between 50 and 300")
}

func TestSpecService_UpdateSpecGuards(t *testing.T) {
	specID := uuid.New()
	other := uuid.New()
	repo := mockRepo{getByIDFn: func(context.Context, uuid.UUID) (*domain.Spec, error) {
		return &domain.Spec{ID: specID, ProducerID: uuid.New()}, nil
	}}
	svc := NewSpecService(repo)

	err := svc.UpdateSpec(context.Background(), &domain.Spec{ID: specID, Title: "x", BasePrice: 1}, other)
	require.EqualError(t, err, "unauthorized: you can only update your own specs")

	svc = NewSpecService(mockRepo{getByIDFn: func(context.Context, uuid.UUID) (*domain.Spec, error) { return nil, nil }})
	err = svc.UpdateSpec(context.Background(), &domain.Spec{ID: specID, Title: "x", BasePrice: 1}, other)
	require.EqualError(t, err, "spec not found")

	svc = NewSpecService(mockRepo{getByIDFn: func(context.Context, uuid.UUID) (*domain.Spec, error) { return nil, errors.New("db") }})
	err = svc.UpdateSpec(context.Background(), &domain.Spec{ID: specID, Title: "x", BasePrice: 1}, other)
	require.EqualError(t, err, "db")
}
