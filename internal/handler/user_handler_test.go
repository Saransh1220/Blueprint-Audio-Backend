package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUserHandler_UpdateProfile(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewUserHandler(svc, fileSvc)
	userID := uuid.New()

	req := httptest.NewRequest(http.MethodPatch, "/users/profile", bytes.NewBufferString("{bad"))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	w := httptest.NewRecorder()
	h.UpdateProfile(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	bio := "new bio"
	payload, _ := json.Marshal(dto.UpdateProfileRequest{Bio: &bio})
	req = httptest.NewRequest(http.MethodPatch, "/users/profile", bytes.NewBuffer(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(nil).Once()
	svc.On("GetPublicProfile", mock.Anything, userID).Return(&dto.PublicUserResponse{ID: userID.String()}, nil).Once()
	w = httptest.NewRecorder()
	h.UpdateProfile(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodPatch, "/users/profile", bytes.NewBuffer(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(errors.New("db")).Once()
	w = httptest.NewRecorder()
	h.UpdateProfile(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUserHandler_GetPublicProfile(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewUserHandler(svc, fileSvc)
	userID := uuid.New()
	avatar := "http://storage/bucket/avatar.jpg"
	profile := &dto.PublicUserResponse{ID: userID.String(), AvatarURL: &avatar}

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
	fileSvc := new(mocks.MockFileService)
	h := handler.NewUserHandler(svc, fileSvc)
	userID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/users/profile/avatar", nil)
	w := httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Multipart without avatar field
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/users/profile/avatar", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	w = httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Service error while loading current profile
	b.Reset()
	mw = multipart.NewWriter(&b)
	fw, _ := mw.CreateFormFile("avatar", "a.jpg")
	_, _ = fw.Write([]byte("not-an-image-but-upload-mock-does-not-care"))
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/users/profile/avatar", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	svc.On("GetPublicProfile", mock.Anything, userID).Return(nil, errors.New("db")).Once()
	w = httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUserHandler_UploadAvatarSuccessAndRollback(t *testing.T) {
	svc := new(mockUserService)
	fileSvc := new(mocks.MockFileService)
	h := handler.NewUserHandler(svc, fileSvc)
	userID := uuid.New()
	oldAvatar := "http://storage/bucket/old.jpg"
	newAvatar := "http://storage/bucket/new.jpg"

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, _ := mw.CreateFormFile("avatar", "a.jpg")
	_, _ = fw.Write([]byte("payload"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/users/profile/avatar", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))

	svc.On("GetPublicProfile", mock.Anything, userID).Return(&dto.PublicUserResponse{ID: userID.String(), AvatarURL: &oldAvatar}, nil).Once()
	fileSvc.On("GetKeyFromUrl", oldAvatar).Return("old.jpg", nil).Once()
	fileSvc.On("Delete", mock.Anything, "old.jpg").Return(nil).Once()
	fileSvc.On("Upload", mock.Anything, mock.Anything, mock.Anything, "avatars").Return(newAvatar, "new.jpg", nil).Once()
	svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(nil).Once()
	svc.On("GetPublicProfile", mock.Anything, userID).Return(&dto.PublicUserResponse{ID: userID.String(), AvatarURL: &newAvatar}, nil).Once()
	fileSvc.On("GetKeyFromUrl", newAvatar).Return("new.jpg", nil).Once()
	fileSvc.On("GetPresignedURL", mock.Anything, "new.jpg", mock.Anything).Return("signed-new", nil).Once()

	w := httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "signed-new")

	// Rollback path
	b.Reset()
	mw = multipart.NewWriter(&b)
	fw, _ = mw.CreateFormFile("avatar", "a.jpg")
	_, _ = fw.Write([]byte("payload"))
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/users/profile/avatar", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))

	svc.On("GetPublicProfile", mock.Anything, userID).Return(&dto.PublicUserResponse{ID: userID.String()}, nil).Once()
	fileSvc.On("Upload", mock.Anything, mock.Anything, mock.Anything, "avatars").Return(newAvatar, "new.jpg", nil).Once()
	svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(errors.New("db")).Once()
	fileSvc.On("GetKeyFromUrl", newAvatar).Return("new.jpg", nil).Once()
	fileSvc.On("Delete", mock.Anything, "new.jpg").Return(nil).Once()

	w = httptest.NewRecorder()
	h.UploadAvatar(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
