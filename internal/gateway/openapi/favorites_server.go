package openapi

import (
	"context"
	"errors"
	"log"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	analyticsApp "github.com/saransh1220/blueprint-audio/internal/modules/analytics/application"
	analyticsDomain "github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

// FavoritesServer implements StrictServerInterface for the favorites endpoints.
// It bridges the oapi-codegen generated contract to the existing AnalyticsService.
type FavoritesServer struct {
	analytics analyticsApp.AnalyticsService
}

// NewFavoritesServer creates a FavoritesServer.
func NewFavoritesServer(analytics analyticsApp.AnalyticsService) *FavoritesServer {
	return &FavoritesServer{analytics: analytics}
}

// ListMyFavorites implements StrictServerInterface.
// Security: The route must be registered behind RequireAuth. This method also
// independently validates that a userID is present in the context, returning
// 401 if it is not — providing defence in depth.
func (s *FavoritesServer) ListMyFavorites(ctx context.Context, request ListMyFavoritesRequestObject) (ListMyFavoritesResponseObject, error) {
	// --- Auth: extract authenticated user ID from context ---
	userID, ok := ctx.Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return ListMyFavorites401JSONResponse{
			UnauthorizedJSONResponse: UnauthorizedJSONResponse{
				Error: "missing or invalid authorization",
			},
		}, nil
	}

	// --- Parse params ---
	limit := 20
	if request.Params.Limit != nil && *request.Params.Limit > 0 {
		limit = *request.Params.Limit
		if limit > 50 {
			limit = 50
		}
	}

	var encodedCursor *string
	if request.Params.Cursor != nil && *request.Params.Cursor != "" {
		encodedCursor = request.Params.Cursor
	}

	// --- Resolve display currency from Cloudflare / fallback headers ---
	displayCurrency := resolveDisplayCurrency(request.Params.CFIPCountry, request.Params.XCountryCode)

	// --- Call service ---
	page, err := s.analytics.ListMyFavorites(ctx, userID, limit, encodedCursor)
	if err != nil {
		// Distinguish invalid cursor (400) from internal errors (500).
		if errors.Is(err, analyticsApp.ErrInvalidCursor) {
			return ListMyFavorites400JSONResponse{
				BadRequestJSONResponse: BadRequestJSONResponse{
					Error:   "invalid cursor",
					Details: strPtr("the provided pagination cursor is invalid or malformed"),
				},
			}, nil
		}
		log.Printf("[FavoritesServer] ListMyFavorites userID=%s: %v", userID, err)
		return ListMyFavorites500JSONResponse{
			InternalErrorJSONResponse: InternalErrorJSONResponse{
				Error: "internal server error",
			},
		}, nil
	}

	// --- Map domain page → generated openapi response ---
	return mapFavoritePage(page, displayCurrency), nil
}

// mapFavoritePage converts a domain.FavoritePage into the generated ListMyFavorites200JSONResponse.
func mapFavoritePage(page *analyticsDomain.FavoritePage, displayCurrency string) ListMyFavorites200JSONResponse {
	items := make([]FavoriteItem, 0, len(page.Items))
	for _, fi := range page.Items {
		items = append(items, FavoriteItem{
			FavoritedAt: fi.FavoritedAt,
			Spec:        mapSpec(&fi.Spec, displayCurrency),
		})
	}

	resp := FavoritePage{
		Items:   items,
		HasMore: page.HasMore,
	}

	// Encode the next cursor if the service populated it.
	if page.HasMore && page.NextCursor != nil {
		encoded, err := analyticsApp.EncodeFavoriteCursor(page.NextCursor)
		if err != nil {
			log.Printf("[FavoritesServer] mapFavoritePage: failed to encode cursor: %v", err)
		} else {
			resp.NextCursor = &encoded
		}
	}

	return ListMyFavorites200JSONResponse(resp)
}

// mapSpec converts a catalog domain Spec into the generated openapi Spec type.
func mapSpec(s *catalogDomain.Spec, displayCurrency string) Spec {
	priceMoney := money.DisplayPrice(s.BasePrice, s.PriceCurrency, s.PriceCurrency)
	displayPriceMoney := money.DisplayPrice(s.BasePrice, s.PriceCurrency, displayCurrency)

	spec := Spec{
		Id:           openapi_types.UUID(s.ID),
		ProducerId:   openapi_types.UUID(s.ProducerID),
		ProducerName: s.ProducerName,
		Title:        s.Title,
		Category:     SpecCategory(s.Category),
		Type:         string(s.Type),
		Bpm:          s.BPM,
		Key:          s.Key,
		Description:  s.Description,
		ImageUrl:     s.ImageUrl,
		PreviewUrl:   s.PreviewUrl,
		Price:        s.BasePrice,
		PriceMoney: Money{
			AmountMinor: int64(priceMoney.AmountMinor),
			AmountMajor: priceMoney.AmountMajor,
			Currency:    priceMoney.Currency,
		},
		DisplayPriceMoney: Money{
			AmountMinor: int64(displayPriceMoney.AmountMinor),
			AmountMajor: displayPriceMoney.AmountMajor,
			Currency:    displayPriceMoney.Currency,
		},
		Duration:         s.Duration,
		FreeMp3Enabled:   s.FreeMp3Enabled,
		ProcessingStatus: SpecProcessingStatus(s.ProcessingStatus),
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
		ShortCode:        s.ShortCode,
		Slug:             s.Slug,
	}

	// Optional array fields — only set when non-empty to keep JSON clean.
	if len(s.Moods) > 0 {
		moods := []string(s.Moods)
		spec.Moods = &moods
	}
	if len(s.Instruments) > 0 {
		instruments := []string(s.Instruments)
		spec.Instruments = &instruments
	}
	if len(s.Tags) > 0 {
		tags := []string(s.Tags)
		spec.Tags = &tags
	}
	if len(s.WaveformPeaks) > 0 {
		peaks := []int64(s.WaveformPeaks)
		spec.WaveformPeaks = &peaks
	}

	if len(s.Licenses) > 0 {
		licenses := make([]LicenseOption, 0, len(s.Licenses))
		for _, l := range s.Licenses {
			lPriceMoney := money.DisplayPrice(l.Price, l.PriceCurrency, l.PriceCurrency)
			lDisplayPriceMoney := money.DisplayPrice(l.Price, l.PriceCurrency, displayCurrency)
			licenses = append(licenses, LicenseOption{
				Id:     openapi_types.UUID(l.ID),
				SpecId: openapi_types.UUID(l.SpecID),
				Type:   LicenseOptionType(l.LicenseType),
				Name:   l.Name,
				Price:  l.Price,
				PriceMoney: Money{
					AmountMinor: int64(lPriceMoney.AmountMinor),
					AmountMajor: lPriceMoney.AmountMajor,
					Currency:    lPriceMoney.Currency,
				},
				DisplayPriceMoney: Money{
					AmountMinor: int64(lDisplayPriceMoney.AmountMinor),
					AmountMajor: lDisplayPriceMoney.AmountMajor,
					Currency:    lDisplayPriceMoney.Currency,
				},
				Features:  []string(l.Features),
				FileTypes: []string(l.FileTypes),
				CreatedAt: l.CreatedAt,
				UpdatedAt: l.UpdatedAt,
			})
		}
		spec.Licenses = &licenses
	}

	if len(s.Genres) > 0 {
		genres := make([]Genre, 0, len(s.Genres))
		for _, g := range s.Genres {
			genres = append(genres, Genre{
				Id:        openapi_types.UUID(g.ID),
				Name:      g.Name,
				Slug:      g.Slug,
				CreatedAt: g.CreatedAt,
			})
		}
		spec.Genres = &genres
	}

	return spec
}

// resolveDisplayCurrency determines the display currency from the request headers.
// IN country code → INR; everything else → USD.
func resolveDisplayCurrency(cfCountry *CloudflareCountry, xCountry *CountryCode) string {
	if cfCountry != nil && *cfCountry == "IN" {
		return money.CurrencyINR
	}
	if xCountry != nil && *xCountry == "IN" {
		return money.CurrencyINR
	}
	return money.CurrencyUSD
}

// strPtr is a small convenience helper.
func strPtr(s string) *string {
	return &s
}

// Ensure compile-time satisfaction of the generated interface.
var _ StrictServerInterface = (*FavoritesServer)(nil)
