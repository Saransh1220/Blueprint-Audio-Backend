package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSpecService_CreateSpecValidation(t *testing.T) {
	ctx := context.Background()
	repo := new(mockSpecRepository)
	svc := service.NewSpecService(repo)

	err := svc.CreateSpec(ctx, &domain.Spec{BasePrice: 100})
	assert.EqualError(t, err, "title is required")

	err = svc.CreateSpec(ctx, &domain.Spec{Title: "x", BasePrice: -1})
	assert.EqualError(t, err, "price cannot be negative")

	err = svc.CreateSpec(ctx, &domain.Spec{Title: "x", BasePrice: 1, Category: domain.CategoryBeat, BPM: 49})
	assert.EqualError(t, err, "BPM must be between 60 and 200")

	wav := "wav"
	err = svc.CreateSpec(ctx, &domain.Spec{Title: "x", BasePrice: 1, Category: domain.CategoryBeat, BPM: 100, WavUrl: &wav})
	assert.EqualError(t, err, "stems file is mandatory for beats")
}

func TestSpecService_CreateSpecSuccessAndProxy(t *testing.T) {
	ctx := context.Background()
	repo := new(mockSpecRepository)
	svc := service.NewSpecService(repo)
	wav := "wav"
	stems := "stems"
	spec := &domain.Spec{Title: "x", BasePrice: 10, Category: domain.CategoryBeat, BPM: 120, WavUrl: &wav, StemsUrl: &stems}

	repo.On("Create", ctx, spec).Return(nil)
	err := svc.CreateSpec(ctx, spec)
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestSpecService_UpdateSpec(t *testing.T) {
	ctx := context.Background()
	repo := new(mockSpecRepository)
	svc := service.NewSpecService(repo)
	specID := uuid.New()
	ownerID := uuid.New()
	otherID := uuid.New()

	repo.On("GetByID", ctx, specID).Return(nil, errors.New("db")).Once()
	err := svc.UpdateSpec(ctx, &domain.Spec{ID: specID}, ownerID)
	assert.EqualError(t, err, "db")

	repo.On("GetByID", ctx, specID).Return((*domain.Spec)(nil), nil).Once()
	err = svc.UpdateSpec(ctx, &domain.Spec{ID: specID}, ownerID)
	assert.EqualError(t, err, "spec not found")

	repo.On("GetByID", ctx, specID).Return(&domain.Spec{ID: specID, ProducerID: otherID}, nil).Once()
	err = svc.UpdateSpec(ctx, &domain.Spec{ID: specID, Title: "t", BasePrice: 10}, ownerID)
	assert.EqualError(t, err, "unauthorized: you can only update your own specs")

	repo.On("GetByID", ctx, specID).Return(&domain.Spec{ID: specID, ProducerID: ownerID}, nil).Once()
	repo.On("Update", ctx, mock.AnythingOfType("*domain.Spec")).Return(nil).Once()
	err = svc.UpdateSpec(ctx, &domain.Spec{ID: specID, ProducerID: otherID, Title: "ok", BasePrice: 10, Category: domain.CategorySample}, ownerID)
	require.NoError(t, err)
	repo.AssertCalled(t, "Update", ctx, mock.MatchedBy(func(s *domain.Spec) bool {
		return s.ProducerID == ownerID
	}))
}

func TestSpecService_ListDeleteAndGetUserSpecs(t *testing.T) {
	ctx := context.Background()
	repo := new(mockSpecRepository)
	svc := service.NewSpecService(repo)
	producerID := uuid.New()
	specID := uuid.New()

	filter := domain.SpecFilter{Limit: 20}
	repo.On("List", ctx, filter).Return([]domain.Spec{{ID: specID}}, 1, nil)
	_, total, err := svc.ListSpecs(ctx, filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)

	repo.On("Delete", ctx, specID, producerID).Return(nil)
	assert.NoError(t, svc.DeleteSpec(ctx, specID, producerID))

	repo.On("ListByUserID", ctx, producerID, 20, 0).Return([]domain.Spec{}, 0, nil).Once()
	_, _, err = svc.GetUserSpecs(ctx, producerID, 0)
	assert.NoError(t, err)
}
