package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/user/application"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock Services
type mockUserService struct {
	mock.Mock
}

func (m *mockUserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req application.UpdateProfileRequest) error {
	args := m.Called(ctx, userID, req)
	return args.Error(0)
}

func (m *mockUserService) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*application.PublicUserResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*application.PublicUserResponse), args.Error(1)
}

type mockFileService struct {
	mock.Mock
}

func (m *mockFileService) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	args := m.Called(ctx, file, header, folder)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *mockFileService) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, expiration)
	return args.String(0), args.Error(1)
}

func (m *mockFileService) GetKeyFromUrl(fileUrl string) (string, error) {
	args := m.Called(fileUrl)
	return args.String(0), args.Error(1)
}

func (m *mockFileService) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func TestUserHandler_UpdateProfile(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mockFileService)
	h := user_http.NewUserHandler(svc, fileSvc)

	t.Cleanup(func() {
		svc.AssertExpectations(t)
		fileSvc.AssertExpectations(t)
	})
	userID := uuid.New()

	// Bad Request (Invalid JSON)
	req := httptest.NewRequest(http.MethodPatch, "/users/profile", bytes.NewBufferString("{bad"))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	w := httptest.NewRecorder()
	h.UpdateProfile(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Success
	bio := "new bio"
	payload, _ := json.Marshal(application.UpdateProfileRequest{Bio: &bio})
	req = httptest.NewRequest(http.MethodPatch, "/users/profile", bytes.NewBuffer(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(nil).Once()
	svc.On("GetPublicProfile", mock.Anything, userID).Return(&application.PublicUserResponse{ID: userID.String(), Bio: &bio}, nil).Once()

	w = httptest.NewRecorder()
	h.UpdateProfile(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Internal Server Error
	req = httptest.NewRequest(http.MethodPatch, "/users/profile", bytes.NewBuffer(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(errors.New("db error")).Once()

	w = httptest.NewRecorder()
	h.UpdateProfile(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUserHandler_GetPublicProfile(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mockFileService)
	h := user_http.NewUserHandler(svc, fileSvc)

	t.Cleanup(func() {
		svc.AssertExpectations(t)
		fileSvc.AssertExpectations(t)
	})
	userID := uuid.New()
	avatar := "http://storage/bucket/avatar.jpg"
	profile := &application.PublicUserResponse{ID: userID.String(), AvatarURL: &avatar}

	req := httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/public", nil)
	req.SetPathValue("id", userID.String())

	svc.On("GetPublicProfile", mock.Anything, userID).Return(profile, nil).Once()
	fileSvc.On("GetKeyFromUrl", avatar).Return("avatar.jpg", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "avatar.jpg", mock.Anything).Return("signed-avatar", nil).Once()

	w := httptest.NewRecorder()
	h.GetPublicProfile(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "signed-avatar")
}

func TestUserHandler_UploadAvatar(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mockFileService)
	h := user_http.NewUserHandler(svc, fileSvc)

	t.Cleanup(func() {
		svc.AssertExpectations(t)
		fileSvc.AssertExpectations(t)
	})
	userID := uuid.New()

	// Unauthorized
	req := httptest.NewRequest(http.MethodPost, "/users/profile/avatar", nil)
	w := httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Success
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	part, _ := mw.CreateFormFile("avatar", "test.jpg")
	part.Write([]byte("image content"))
	mw.Close()

	req = httptest.NewRequest(http.MethodPost, "/users/profile/avatar", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))

	oldAvatar := "http://old/avatar.jpg"
	newAvatar := "http://new/avatar.jpg"

	// Mock sequence
	svc.On("GetPublicProfile", mock.Anything, userID).Return(&application.PublicUserResponse{ID: userID.String(), AvatarURL: &oldAvatar}, nil).Once()
	fileSvc.On("GetKeyFromUrl", oldAvatar).Return("old_key", nil).Once()
	fileSvc.On("Delete", mock.Anything, "old_key").Return(nil).Once()
	fileSvc.On("Upload", mock.Anything, mock.Anything, mock.Anything, "avatars").Return(newAvatar, "new_key", nil).Once()
	svc.On("UpdateProfile", mock.Anything, userID, mock.MatchedBy(func(r application.UpdateProfileRequest) bool {
		return r.AvatarURL != nil && *r.AvatarURL == newAvatar
	})).Return(nil).Once()
	svc.On("GetPublicProfile", mock.Anything, userID).Return(&application.PublicUserResponse{ID: userID.String(), AvatarURL: &newAvatar}, nil).Once()
	fileSvc.On("GetKeyFromUrl", newAvatar).Return("new_key", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "new_key", mock.Anything).Return("signed-new-avatar", nil).Once()

	w = httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "signed-new-avatar")
}

func TestUserHandler_GetPublicProfile_ErrorBranches(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mockFileService)
	h := user_http.NewUserHandler(svc, fileSvc)

	t.Cleanup(func() {
		svc.AssertExpectations(t)
		fileSvc.AssertExpectations(t)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/bad/public", nil)
	req.SetPathValue("id", "bad")
	w := httptest.NewRecorder()
	h.GetPublicProfile(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	userID := uuid.New()
	req = httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/public", nil)
	req.SetPathValue("id", userID.String())
	svc.On("GetPublicProfile", mock.Anything, userID).Return(nil, errors.New("db")).Once()
	w = httptest.NewRecorder()
	h.GetPublicProfile(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUserHandler_UpdateProfile_Unauthorized(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mockFileService)
	h := user_http.NewUserHandler(svc, fileSvc)

	t.Cleanup(func() {
		svc.AssertExpectations(t)
		fileSvc.AssertExpectations(t)
	})

	req := httptest.NewRequest(http.MethodPatch, "/users/profile", bytes.NewBufferString(`{"bio":"x"}`))
	w := httptest.NewRecorder()
	h.UpdateProfile(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserHandler_UploadAvatar_RollbackOnUpdateError(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mockFileService)
	h := user_http.NewUserHandler(svc, fileSvc)

	t.Cleanup(func() {
		svc.AssertExpectations(t)
		fileSvc.AssertExpectations(t)
	})
	userID := uuid.New()

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	part, _ := mw.CreateFormFile("avatar", "test.jpg")
	_, _ = part.Write([]byte("image content"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/users/profile/avatar", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))

	newAvatar := "http://new/avatar.jpg"
	svc.On("GetPublicProfile", mock.Anything, userID).Return(&application.PublicUserResponse{ID: userID.String()}, nil).Once()
	fileSvc.On("Upload", mock.Anything, mock.Anything, mock.Anything, "avatars").Return(newAvatar, "new_key", nil).Once()
	svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(errors.New("update fail")).Once()
	fileSvc.On("GetKeyFromUrl", newAvatar).Return("new_key", nil).Once()
	fileSvc.On("Delete", mock.Anything, "new_key").Return(nil).Once()

	w := httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
