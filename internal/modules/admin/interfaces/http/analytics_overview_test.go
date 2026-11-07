package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsOverview_DetailedBreakdown(t *testing.T) {
	t.Run("multi-currency with converted and unconverted orders", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()

		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(15))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users WHERE role = 'producer'").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM specs").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(30))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM orders").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM orders WHERE status = 'paid'").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))

		revRows := sqlmock.NewRows([]string{"currency", "amount_minor", "order_count"}).
			AddRow("INR", int64(300000), 5).
			AddRow("USD", int64(10000), 3)
		mock.ExpectQuery("SELECT currency, COALESCE").WillReturnRows(revRows)

		mock.ExpectQuery("SELECT COALESCE\\(SUM\\(ROUND\\(amount \\* report_fx_rate\\)\\)").WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(int64(1131200)))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM orders WHERE status = 'paid' AND report_fx_rate IS NULL").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/admin/analytics/overview", nil)
		h.AnalyticsOverview(w, r)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, float64(15), resp["users"])
		assert.Equal(t, float64(4), resp["producers"])
		assert.Equal(t, float64(30), resp["specs"])
		assert.Equal(t, float64(10), resp["orders"])
		assert.Equal(t, float64(8), resp["paid_orders"])
		assert.Equal(t, float64(1131200), resp["revenue"])
		assert.Equal(t, float64(1131200), resp["converted_revenue"])
		assert.Equal(t, float64(2), resp["unconverted_paid_orders"])

		revByCurr, ok := resp["revenue_by_currency"].([]any)
		require.True(t, ok)
		require.Len(t, revByCurr, 2)

		inrItem := revByCurr[0].(map[string]any)
		assert.Equal(t, "INR", inrItem["currency"])
		assert.Equal(t, float64(300000), inrItem["amount_minor"])
		assert.Equal(t, float64(5), inrItem["order_count"])

		usdItem := revByCurr[1].(map[string]any)
		assert.Equal(t, "USD", usdItem["currency"])
		assert.Equal(t, float64(10000), usdItem["amount_minor"])
		assert.Equal(t, float64(3), usdItem["order_count"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty orders table returns empty slice and zero counts", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()

		for i := 0; i < 5; i++ {
			mock.ExpectQuery("SELECT COUNT\\(\\*\\)").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		}
		mock.ExpectQuery("SELECT currency, COALESCE").WillReturnRows(sqlmock.NewRows([]string{"currency", "amount_minor", "order_count"}))
		mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\)").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/admin/analytics/overview", nil)
		h.AnalyticsOverview(w, r)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, float64(0), resp["revenue"])
		assert.Equal(t, float64(0), resp["converted_revenue"])
		assert.Equal(t, float64(0), resp["unconverted_paid_orders"])

		revByCurr, ok := resp["revenue_by_currency"].([]any)
		require.True(t, ok)
		assert.Empty(t, revByCurr)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
