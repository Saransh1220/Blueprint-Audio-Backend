package openapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	analyticsApp "github.com/saransh1220/blueprint-audio/internal/modules/analytics/application"
	analyticsDomain "github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAnalytics struct {
	analyticsApp.AnalyticsService
	page      *analyticsDomain.FavoritePage
	err       error
	gotUser   uuid.UUID
	gotLimit  int
	gotCursor *string
	called    bool
}

func (f *fakeAnalytics) ListMyFavorites(_ context.Context, userID uuid.UUID, limit int, cursor *string) (*analyticsDomain.FavoritePage, error) {
	f.called = true
	f.gotUser = userID
	f.gotLimit = limit
	f.gotCursor = cursor
	return f.page, f.err
}

func authCtx(id uuid.UUID) context.Context {
	return context.WithValue(context.Background(), middleware.ContextKeyUserId, id)
}

func TestListMyFavorites_Unauthenticated(t *testing.T) {
	f := &fakeAnalytics{}
	s := NewFavoritesServer(f)

	for name, ctx := range map[string]context.Context{
		"missing":    context.Background(),
		"nil uuid":   authCtx(uuid.Nil),
		"wrong type": context.WithValue(context.Background(), middleware.ContextKeyUserId, "abc"),
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := s.ListMyFavorites(ctx, ListMyFavoritesRequestObject{})
			require.NoError(t, err)
			r, ok := resp.(ListMyFavorites401JSONResponse)
			require.True(t, ok)
			assert.Equal(t, "missing or invalid authorization", r.Error)
			assert.False(t, f.called)
		})
	}
}

func TestListMyFavorites_LimitAndCursorParsing(t *testing.T) {
	zero, neg, big, mid := 0, -3, 500, 7
	empty, cur := "", "abc"
	tests := []struct {
		name       string
		limit      *int
		cursor     *string
		wantLimit  int
		wantCursor *string
	}{
		{"defaults", nil, nil, 20, nil},
		{"zero limit", &zero, nil, 20, nil},
		{"negative limit", &neg, nil, 20, nil},
		{"clamped", &big, nil, 50, nil},
		{"custom", &mid, nil, 7, nil},
		{"empty cursor ignored", nil, &empty, 20, nil},
		{"cursor passed", nil, &cur, 20, &cur},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid := uuid.New()
			f := &fakeAnalytics{page: &analyticsDomain.FavoritePage{}}
			s := NewFavoritesServer(f)
			resp, err := s.ListMyFavorites(authCtx(uid), ListMyFavoritesRequestObject{
				Params: ListMyFavoritesParams{Limit: tt.limit, Cursor: tt.cursor},
			})
			require.NoError(t, err)
			_, ok := resp.(ListMyFavorites200JSONResponse)
			require.True(t, ok)
			assert.Equal(t, uid, f.gotUser)
			assert.Equal(t, tt.wantLimit, f.gotLimit)
			assert.Equal(t, tt.wantCursor, f.gotCursor)
		})
	}
}

func TestListMyFavorites_ServiceErrors(t *testing.T) {
	t.Run("invalid cursor", func(t *testing.T) {
		f := &fakeAnalytics{err: fmt.Errorf("%w: bad", analyticsApp.ErrInvalidCursor)}
		resp, err := NewFavoritesServer(f).ListMyFavorites(authCtx(uuid.New()), ListMyFavoritesRequestObject{})
		require.NoError(t, err)
		r, ok := resp.(ListMyFavorites400JSONResponse)
		require.True(t, ok)
		assert.Equal(t, "invalid cursor", r.Error)
		require.NotNil(t, r.Details)
		assert.Contains(t, *r.Details, "cursor")
	})
	t.Run("internal error", func(t *testing.T) {
		f := &fakeAnalytics{err: errors.New("db down")}
		resp, err := NewFavoritesServer(f).ListMyFavorites(authCtx(uuid.New()), ListMyFavoritesRequestObject{})
		require.NoError(t, err)
		r, ok := resp.(ListMyFavorites500JSONResponse)
		require.True(t, ok)
		assert.Equal(t, "internal server error", r.Error)
	})
}

func fullSpec() catalogDomain.Spec {
	now := time.Now().UTC()
	specID := uuid.New()
	slug, code := "my-beat", "AB12"
	return catalogDomain.Spec{
		ID: specID, ProducerID: uuid.New(), ProducerName: "prod", Title: "Beat",
		Category: catalogDomain.Category("beat"), Type: "WAV", BPM: 140, Key: "Am",
		ImageUrl: "img", PreviewUrl: "prev", BasePrice: 10, PriceCurrency: "USD",
		Description: "desc", Duration: 120, FreeMp3Enabled: true,
		CreatedAt: now, UpdatedAt: now,
		Moods: pq.StringArray{"dark"}, Instruments: pq.StringArray{"808"},
		Tags: pq.StringArray{"trap"}, WaveformPeaks: pq.Int64Array{1, 2, 3},
		Slug: &slug, ShortCode: &code,
		ProcessingStatus: catalogDomain.ProcessingStatusCompleted,
		Licenses: []catalogDomain.LicenseOption{{
			ID: uuid.New(), SpecID: specID, LicenseType: catalogDomain.LicenseType("basic"),
			Name: "Basic", Price: 10, PriceCurrency: "USD",
			Features: pq.StringArray{"f"}, FileTypes: pq.StringArray{"mp3"},
			CreatedAt: now, UpdatedAt: now,
		}},
		Genres: []catalogDomain.Genre{{ID: uuid.New(), Name: "Trap", Slug: "trap", CreatedAt: now}},
	}
}

func TestListMyFavorites_SuccessMapping(t *testing.T) {
	spec := fullSpec()
	favAt := time.Now().UTC()
	next := &analyticsDomain.FavoriteCursor{FavoritedAt: favAt, SpecID: spec.ID}
	f := &fakeAnalytics{page: &analyticsDomain.FavoritePage{
		Items:      []analyticsDomain.FavoriteItem{{Spec: spec, FavoritedAt: favAt}},
		HasMore:    true,
		NextCursor: next,
	}}
	in := "IN"
	resp, err := NewFavoritesServer(f).ListMyFavorites(authCtx(uuid.New()), ListMyFavoritesRequestObject{
		Params: ListMyFavoritesParams{CFIPCountry: &in},
	})
	require.NoError(t, err)
	r, ok := resp.(ListMyFavorites200JSONResponse)
	require.True(t, ok)
	assert.True(t, r.HasMore)
	require.NotNil(t, r.NextCursor)
	decoded, err := analyticsApp.DecodeFavoriteCursor(*r.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, spec.ID, decoded.SpecID)

	require.Len(t, r.Items, 1)
	got := r.Items[0]
	assert.True(t, got.FavoritedAt.Equal(favAt))
	assert.Equal(t, spec.ID, uuid.UUID(got.Spec.Id))
	assert.Equal(t, "Beat", got.Spec.Title)
	assert.Equal(t, "WAV", got.Spec.Type)
	assert.Equal(t, "USD", got.Spec.PriceMoney.Currency)
	assert.Equal(t, "INR", got.Spec.DisplayPriceMoney.Currency)
	require.NotNil(t, got.Spec.Moods)
	assert.Equal(t, []string{"dark"}, *got.Spec.Moods)
	require.NotNil(t, got.Spec.Instruments)
	require.NotNil(t, got.Spec.Tags)
	require.NotNil(t, got.Spec.WaveformPeaks)
	assert.Equal(t, []int64{1, 2, 3}, *got.Spec.WaveformPeaks)
	require.NotNil(t, got.Spec.Licenses)
	require.Len(t, *got.Spec.Licenses, 1)
	lic := (*got.Spec.Licenses)[0]
	assert.Equal(t, "Basic", lic.Name)
	assert.Equal(t, "INR", lic.DisplayPriceMoney.Currency)
	assert.Equal(t, "USD", lic.PriceMoney.Currency)
	assert.Equal(t, []string{"mp3"}, lic.FileTypes)
	require.NotNil(t, got.Spec.Genres)
	require.Len(t, *got.Spec.Genres, 1)
	assert.Equal(t, "trap", (*got.Spec.Genres)[0].Slug)
}

func TestMapFavoritePage_MinimalAndNoCursor(t *testing.T) {
	spec := catalogDomain.Spec{ID: uuid.New(), PriceCurrency: "USD", BasePrice: 5}
	page := &analyticsDomain.FavoritePage{
		Items:   []analyticsDomain.FavoriteItem{{Spec: spec}},
		HasMore: true, // but no NextCursor
	}
	r := mapFavoritePage(page, "USD")
	assert.True(t, r.HasMore)
	assert.Nil(t, r.NextCursor)
	require.Len(t, r.Items, 1)
	s := r.Items[0].Spec
	assert.Nil(t, s.Moods)
	assert.Nil(t, s.Instruments)
	assert.Nil(t, s.Tags)
	assert.Nil(t, s.WaveformPeaks)
	assert.Nil(t, s.Licenses)
	assert.Nil(t, s.Genres)

	// HasMore false ignores cursor
	page2 := &analyticsDomain.FavoritePage{NextCursor: &analyticsDomain.FavoriteCursor{}}
	r2 := mapFavoritePage(page2, "USD")
	assert.Nil(t, r2.NextCursor)
	assert.Empty(t, r2.Items)
	assert.NotNil(t, r2.Items)
}

func TestResolveDisplayCurrency(t *testing.T) {
	in, us := "IN", "US"
	tests := []struct {
		name string
		cf   *string
		x    *string
		want string
	}{
		{"none", nil, nil, "USD"},
		{"cf IN", &in, nil, "INR"},
		{"x IN", nil, &in, "INR"},
		{"cf US x IN", &us, &in, "INR"},
		{"both US", &us, &us, "USD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveDisplayCurrency(tt.cf, tt.x))
		})
	}
}

func TestStrPtr(t *testing.T) {
	p := strPtr("x")
	require.NotNil(t, p)
	assert.Equal(t, "x", *p)
}
