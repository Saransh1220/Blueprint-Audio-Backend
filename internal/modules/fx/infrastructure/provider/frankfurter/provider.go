package frankfurter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
)

var currencyCodeRegex = regexp.MustCompile(`^[A-Z]{3}$`)

type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type Provider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewProvider(cfg Config) (*Provider, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.frankfurter.dev/v1/latest"
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid provider URL: %w", err)
	}

	// HTTPS enforcement (allow http only for local test environments)
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return nil, errors.New("provider URL must use HTTPS")
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &Provider{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: client,
	}, nil
}

func (p *Provider) Name() string {
	return "frankfurter"
}

type frankfurterResponse struct {
	Amount json.Number            `json:"amount"`
	Base   string                 `json:"base"`
	Date   string                 `json:"date"`
	Rates  map[string]json.Number `json:"rates"`
}

func (p *Provider) Fetch(ctx context.Context, base string, quotes []string) (*domain.ProviderRates, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	if !currencyCodeRegex.MatchString(base) {
		return nil, fmt.Errorf("invalid base currency code: %s", base)
	}

	validQuotes := make([]string, 0, len(quotes))
	for _, q := range quotes {
		q = strings.ToUpper(strings.TrimSpace(q))
		if !currencyCodeRegex.MatchString(q) {
			return nil, fmt.Errorf("invalid quote currency code: %s", q)
		}
		if q != base {
			validQuotes = append(validQuotes, q)
		}
	}

	if len(validQuotes) == 0 {
		return nil, errors.New("no valid quote currencies requested")
	}

	reqURL, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := reqURL.Query()
	q.Set("base", base)
	q.Set("symbols", strings.Join(validQuotes, ","))
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fx provider request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fx provider returned non-200 status: %d", resp.StatusCode)
	}

	// Security: limit response to 1 MB to prevent unbounded memory read
	limitedReader := io.LimitReader(resp.Body, 1<<20)
	decoder := json.NewDecoder(limitedReader)
	decoder.UseNumber()

	var payload frankfurterResponse
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode provider response: %w", err)
	}

	if payload.Base != base {
		return nil, fmt.Errorf("provider returned mismatched base currency: expected %s, got %s", base, payload.Base)
	}

	asOf, err := time.Parse(time.DateOnly, payload.Date)
	if err != nil {
		// Fallback to RFC3339 if full timestamp provided
		asOf, err = time.Parse(time.RFC3339, payload.Date)
		if err != nil {
			return nil, fmt.Errorf("failed to parse as_of date %q: %w", payload.Date, err)
		}
	}

	parsedRates := make(map[string]*big.Rat, len(payload.Rates))
	for quoteCode, rawRate := range payload.Rates {
		quoteCode = strings.ToUpper(strings.TrimSpace(quoteCode))
		rateRat, ok := new(big.Rat).SetString(string(rawRate))
		if !ok || rateRat.Sign() <= 0 {
			return nil, fmt.Errorf("provider returned non-positive or invalid rate for %s: %s", quoteCode, rawRate)
		}
		parsedRates[quoteCode] = rateRat
	}

	return &domain.ProviderRates{
		Base:   base,
		AsOf:   asOf.UTC(),
		Rates:  parsedRates,
		Source: p.Name(),
	}, nil
}
