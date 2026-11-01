package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listCase describes one paginated admin list endpoint.
type listCase struct {
	name   string
	call   func(h *AdminHandler) http.HandlerFunc
	from   string // FROM/JOIN fragment shared by the data and COUNT queries
	search string // WHERE fragment produced by ?q=
	order  string
}

func listCases() []listCase {
	return []listCase{
		{
			name:   "specs",
			call:   func(h *AdminHandler) http.HandlerFunc { return h.ListSpecs },
			from:   `FROM specs s JOIN users u ON s.producer_id = u.id`,
			search: `WHERE \(s.title ILIKE \$1 OR u.email ILIKE \$1 OR COALESCE\(u.display_name, u.name\) ILIKE \$1\)`,
			order:  `ORDER BY s.created_at DESC, s.id`,
		},
		{
			name:   "orders",
			call:   func(h *AdminHandler) http.HandlerFunc { return h.ListOrders },
			from:   `FROM orders o JOIN users u ON o.user_id = u.id JOIN specs s ON o.spec_id = s.id`,
			search: `WHERE \(u.email ILIKE \$1 OR s.title ILIKE \$1\)`,
			order:  `ORDER BY o.created_at DESC, o.id`,
		},
		{
			name:   "licenses",
			call:   func(h *AdminHandler) http.HandlerFunc { return h.ListLicenses },
			from:   `FROM licenses l JOIN users u ON l.user_id = u.id JOIN specs s ON l.spec_id = s.id`,
			search: `WHERE \(u.email ILIKE \$1 OR s.title ILIKE \$1\)`,
			order:  `ORDER BY l.issued_at DESC, l.id`,
		},
		{
			name:   "audit log",
			call:   func(h *AdminHandler) http.HandlerFunc { return h.ListAuditLog },
			from:   `FROM admin_audit_logs a LEFT JOIN users u ON a.actor_id = u.id`,
			search: `WHERE \(a.action ILIKE \$1 OR a.resource_type ILIKE \$1 OR u.email ILIKE \$1\)`,
			order:  `ORDER BY a.created_at DESC, a.id`,
		},
	}
}

func countRows(n int) *sqlmock.Rows { return sqlmock.NewRows([]string{"count"}).AddRow(n) }

func TestAdminLists_PageSizeComesFromTheClient(t *testing.T) {
	for _, tc := range listCases() {
		t.Run(tc.name, func(t *testing.T) {
			h, mock, closeDB := newHandler(t)
			defer closeDB()

			// ?limit=5&page=3 -> LIMIT 5 OFFSET 10, and no WHERE without ?q=
			mock.ExpectQuery(tc.from + ` ` + tc.order + ` LIMIT \$1 OFFSET \$2`).
				WithArgs(5, 10).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(`SELECT COUNT\(\*\) ` + tc.from + `$`).WillReturnRows(countRows(42))

			w := httptest.NewRecorder()
			tc.call(h)(w, httptest.NewRequest(http.MethodGet, "/?limit=5&page=3", nil))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.NoError(t, mock.ExpectationsWereMet())

			var body struct {
				Data     []any `json:"data"`
				Metadata struct {
					Total   int `json:"total"`
					Page    int `json:"page"`
					PerPage int `json:"per_page"`
				} `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, 42, body.Metadata.Total)
			assert.Equal(t, 3, body.Metadata.Page)
			assert.Equal(t, 5, body.Metadata.PerPage)
			assert.NotNil(t, body.Data, "an empty page must serialise as [] not null")
			assert.Empty(t, body.Data)
		})
	}
}

func TestAdminLists_SearchFiltersRowsAndTotal(t *testing.T) {
	for _, tc := range listCases() {
		t.Run(tc.name, func(t *testing.T) {
			h, mock, closeDB := newHandler(t)
			defer closeDB()

			mock.ExpectQuery(tc.from + ` ` + tc.search + ` ` + tc.order + ` LIMIT \$2 OFFSET \$3`).
				WithArgs("%bob%", 10, 0).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			// The total must be counted with the same filter, or pagination
			// would offer pages that do not exist.
			mock.ExpectQuery(`SELECT COUNT\(\*\) ` + tc.from + ` ` + tc.search + `$`).
				WithArgs("%bob%").
				WillReturnRows(countRows(3))

			w := httptest.NewRecorder()
			tc.call(h)(w, httptest.NewRequest(http.MethodGet, "/?q=%20bob%20&limit=10", nil))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAdminLists_LimitIsCappedAndDefaulted(t *testing.T) {
	for _, tc := range []struct {
		query string
		limit int
	}{
		{"/", defaultPageSize},
		{"/?limit=999", maxPageSize},
		{"/?limit=0", defaultPageSize},
		{"/?limit=abc", defaultPageSize},
	} {
		t.Run(tc.query, func(t *testing.T) {
			h, mock, closeDB := newHandler(t)
			defer closeDB()
			mock.ExpectQuery(`FROM specs s .* LIMIT \$1 OFFSET \$2`).
				WithArgs(tc.limit, 0).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnRows(countRows(0))

			w := httptest.NewRecorder()
			h.ListSpecs(w, httptest.NewRequest(http.MethodGet, tc.query, nil))
			require.Equal(t, http.StatusOK, w.Code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAdminLists_CountFailureIsReported(t *testing.T) {
	for _, tc := range listCases() {
		t.Run(tc.name, func(t *testing.T) {
			h, mock, closeDB := newHandler(t)
			defer closeDB()
			mock.ExpectQuery(`SELECT .* LIMIT`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnError(errors.New("db"))

			w := httptest.NewRecorder()
			tc.call(h)(w, httptest.NewRequest(http.MethodGet, "/", nil))
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Contains(t, w.Body.String(), "failed to count")
		})
	}
}

func TestSearchFilterAndPageClause(t *testing.T) {
	where, args := searchFilter(httptest.NewRequest(http.MethodGet, "/", nil), "a", "b")
	assert.Empty(t, where)
	assert.Nil(t, args)

	where, args = searchFilter(httptest.NewRequest(http.MethodGet, "/?q=x", nil), "a", "b")
	assert.Equal(t, ` WHERE (a ILIKE $1 OR b ILIKE $1)`, where)
	assert.Equal(t, []any{"%x%"}, args)

	assert.Equal(t, ` LIMIT $1 OFFSET $2`, pageClause(0))
	assert.Equal(t, ` LIMIT $2 OFFSET $3`, pageClause(1))
}
