package http_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/user/application"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func validLandscapeJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 12, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 12; x++ {
			img.Set(x, y, color.RGBA{R: 25, G: 100, B: 220, A: 255})
		}
	}
	var buffer bytes.Buffer
	assert.NoError(t, jpeg.Encode(&buffer, img, nil))
	return buffer.Bytes()
}

func multipartImageRequest(t *testing.T, field, filename string, data []byte, userID uuid.UUID) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/users/profile/"+field, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
}

func TestUserHandlerUpdateProfileFailsWhenReloadFails(t *testing.T) {
	service := new(mockUserService)
	files := new(mockFileService)
	handler := user_http.NewUserHandler(service, files)
	userID := uuid.New()
	service.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(nil).Once()
	service.On("GetPublicProfile", mock.Anything, userID).Return(nil, errors.New("reload failed")).Once()

	req := httptest.NewRequest(http.MethodPatch, "/users/profile", strings.NewReader(`{"bio":"ok"}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
	response := httptest.NewRecorder()
	handler.UpdateProfile(response, req)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	service.AssertExpectations(t)
}

func TestUserHandlerUploadBannerSuccess(t *testing.T) {
	service := new(mockUserService)
	files := new(mockFileService)
	handler := user_http.NewUserHandler(service, files)
	userID := uuid.New()
	oldURL := "https://storage/old-banner.jpg"
	newURL := "https://storage/new-banner.jpg"

	service.On("GetPublicProfile", mock.Anything, userID).
		Return(&application.PublicUserResponse{ID: userID.String(), BannerURL: &oldURL}, nil).Once()
	files.On("UploadWithKey", mock.Anything, mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "banners/") && strings.HasSuffix(key, ".jpg")
	}), "image/jpeg").Return(newURL, nil).Once()
	service.On("UpdateProfile", mock.Anything, userID, mock.MatchedBy(func(req application.UpdateProfileRequest) bool {
		return req.BannerURL != nil && *req.BannerURL == newURL && req.AvatarURL == nil
	})).Return(nil).Once()
	files.On("GetKeyFromUrl", oldURL).Return("old-banner", nil).Once()
	files.On("Delete", mock.Anything, "old-banner").Return(nil).Once()
	service.On("GetPublicProfile", mock.Anything, userID).
		Return(&application.PublicUserResponse{ID: userID.String(), BannerURL: &newURL}, nil).Once()
	files.On("GetKeyFromUrl", newURL).Return("new-banner", nil).Once()
	files.On("GetPresignedURL", mock.Anything, "new-banner", mock.Anything).Return("signed-banner", nil).Once()

	response := httptest.NewRecorder()
	handler.UploadBanner(response, multipartImageRequest(t, "banner", "banner.jpg", validLandscapeJPEG(t), userID))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "signed-banner")
	service.AssertExpectations(t)
	files.AssertExpectations(t)
}

func TestUserHandlerUploadImageErrorBranches(t *testing.T) {
	userID := uuid.New()

	t.Run("malformed multipart", func(t *testing.T) {
		handler := user_http.NewUserHandler(new(mockUserService), new(mockFileService))
		req := httptest.NewRequest(http.MethodPost, "/users/profile/avatar", strings.NewReader("broken"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
		response := httptest.NewRecorder()
		handler.UploadAvatar(response, req)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("missing file", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		assert.NoError(t, writer.Close())
		req := httptest.NewRequest(http.MethodPost, "/users/profile/avatar", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
		response := httptest.NewRecorder()
		user_http.NewUserHandler(new(mockUserService), new(mockFileService)).UploadAvatar(response, req)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("invalid image", func(t *testing.T) {
		response := httptest.NewRecorder()
		user_http.NewUserHandler(new(mockUserService), new(mockFileService)).UploadAvatar(
			response, multipartImageRequest(t, "avatar", "bad.jpg", []byte("not an image"), userID),
		)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("profile lookup", func(t *testing.T) {
		service := new(mockUserService)
		service.On("GetPublicProfile", mock.Anything, userID).Return(nil, errors.New("lookup failed")).Once()
		response := httptest.NewRecorder()
		user_http.NewUserHandler(service, new(mockFileService)).UploadAvatar(
			response, multipartImageRequest(t, "avatar", "avatar.jpg", validSquareJPEG(t), userID),
		)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		service.AssertExpectations(t)
	})

	t.Run("storage upload", func(t *testing.T) {
		service := new(mockUserService)
		files := new(mockFileService)
		service.On("GetPublicProfile", mock.Anything, userID).Return(&application.PublicUserResponse{}, nil).Once()
		files.On("UploadWithKey", mock.Anything, mock.Anything, mock.Anything, "image/jpeg").Return("", errors.New("upload failed")).Once()
		response := httptest.NewRecorder()
		user_http.NewUserHandler(service, files).UploadAvatar(
			response, multipartImageRequest(t, "avatar", "avatar.jpg", validSquareJPEG(t), userID),
		)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		service.AssertExpectations(t)
		files.AssertExpectations(t)
	})

	t.Run("final profile reload", func(t *testing.T) {
		service := new(mockUserService)
		files := new(mockFileService)
		service.On("GetPublicProfile", mock.Anything, userID).Return(&application.PublicUserResponse{}, nil).Once()
		files.On("UploadWithKey", mock.Anything, mock.Anything, mock.Anything, "image/jpeg").Return("new-url", nil).Once()
		service.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(nil).Once()
		service.On("GetPublicProfile", mock.Anything, userID).Return(nil, errors.New("reload failed")).Once()
		response := httptest.NewRecorder()
		user_http.NewUserHandler(service, files).UploadAvatar(
			response, multipartImageRequest(t, "avatar", "avatar.jpg", validSquareJPEG(t), userID),
		)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		service.AssertExpectations(t)
		files.AssertExpectations(t)
	})
}

func TestUserHandlerSanitizeToleratesStorageErrors(t *testing.T) {
	service := new(mockUserService)
	files := new(mockFileService)
	handler := user_http.NewUserHandler(service, files)
	userID := uuid.New()
	avatar := "bad-avatar"
	banner := "banner"
	service.On("GetPublicProfile", mock.Anything, userID).Return(&application.PublicUserResponse{
		ID: userID.String(), AvatarURL: &avatar, BannerURL: &banner,
	}, nil).Once()
	files.On("GetKeyFromUrl", avatar).Return("", errors.New("bad URL")).Once()
	files.On("GetKeyFromUrl", banner).Return("banner-key", nil).Once()
	files.On("GetPresignedURL", mock.Anything, "banner-key", mock.Anything).Return("", errors.New("sign failed")).Once()

	req := httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/public", nil)
	req.SetPathValue("id", userID.String())
	response := httptest.NewRecorder()
	handler.GetPublicProfile(response, req)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), banner)
	service.AssertExpectations(t)
	files.AssertExpectations(t)
}
