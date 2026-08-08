// SPDX-License-Identifier: GPL-3.0-only

package proton

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
)

// protonDBBaseURL is ProtonDB's public report-summary API — verified live
// (2026-08-08) against appid 1245620 (ELDEN RING), returning
// {tier, bestReportedTier, trendingTier, confidence, score, total}.
const protonDBBaseURL = "https://www.protondb.com/api/v1/reports/summaries"

// protonDBTTL is §9's "ProtonDB ratings" row: 7 d soft, 30 d hard.
var protonDBTTL = cache.TTL{Soft: 7 * 24 * time.Hour, Hard: 30 * 24 * time.Hour}

// ProtonDBRating is one game's community-reported Proton compatibility,
// straight from ProtonDB's own tier scale (e.g. "platinum", "gold",
// "silver", "bronze", "garbage", "borked", "pending") — rigfit doesn't
// reinterpret or re-score it.
type ProtonDBRating struct {
	Tier             string  `json:"tier"`
	BestReportedTier string  `json:"best_reported_tier,omitempty"`
	TrendingTier     string  `json:"trending_tier,omitempty"`
	Confidence       string  `json:"confidence,omitempty"`
	Score            float64 `json:"score,omitempty"`
	Total            int     `json:"total,omitempty"`
	// Available is false when no rating could be obtained (no cache and
	// offline, a fetch failure, or cache older than the 30-day hard TTL)
	// — §2A: "Degrade gracefully if unavailable", never guessed at.
	Available bool `json:"available"`
}

// ProtonDBClient fetches and caches ProtonDB compatibility ratings.
type ProtonDBClient struct {
	httpClient *http.Client
	baseURL    string // overridden in tests to point at an httptest server

	// Explain, when non-nil, is called with the full URL immediately
	// before every rating fetch — §11's --explain flag.
	Explain func(url string)
}

// NewProtonDBClient constructs a ProtonDBClient.
func NewProtonDBClient() *ProtonDBClient {
	return &ProtonDBClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    protonDBBaseURL,
	}
}

func protonDBCacheKey(appID int) string {
	return fmt.Sprintf("protondb-%d.json", appID)
}

// FetchRating returns a game's ProtonDB rating, consulting the cache per
// §9: fresh cache is served as-is; stale cache is used as a fallback if a
// live refresh isn't possible (offline, or the fetch fails); data past
// the 30-day hard TTL is never served — Available is false instead,
// exactly as reqs' hard-expiry-without-stale-ok refusal, but without a
// --stale-ok override, since ProtonDB compatibility can shift with driver
// and Proton releases and stale data here risks being actively wrong
// rather than just outdated.
func (c *ProtonDBClient) FetchRating(ctx context.Context, appID int, offline bool) (ProtonDBRating, error) {
	key := protonDBCacheKey(appID)
	raw, age, cacheErr := cache.ReadFile(key)
	haveCache := cacheErr == nil
	var freshness cache.Freshness
	if haveCache {
		freshness = cache.Classify(age, protonDBTTL)
	}

	if haveCache && freshness == cache.Fresh {
		if r, ok := decodeRating(raw); ok {
			return r, nil
		}
	}

	if offline {
		return staleFallback(raw, haveCache, freshness), nil
	}

	fetched, err := c.fetch(ctx, appID)
	if err != nil {
		return staleFallback(raw, haveCache, freshness), nil
	}
	fetched.Available = true

	body, err := json.Marshal(fetched)
	if err != nil {
		return ProtonDBRating{}, err
	}
	if err := cache.WriteFile(body, key); err != nil {
		return ProtonDBRating{}, err
	}
	return fetched, nil
}

// staleFallback serves cached data only if it's within the hard TTL
// (Stale, not Expired) — the "do not guess" boundary for this data class.
func staleFallback(raw []byte, haveCache bool, freshness cache.Freshness) ProtonDBRating {
	if haveCache && freshness != cache.Expired {
		if r, ok := decodeRating(raw); ok {
			return r
		}
	}
	return ProtonDBRating{Available: false}
}

func decodeRating(raw []byte) (ProtonDBRating, bool) {
	var r ProtonDBRating
	if err := json.Unmarshal(raw, &r); err != nil {
		return ProtonDBRating{}, false
	}
	r.Available = true
	return r, true
}

func (c *ProtonDBClient) fetch(ctx context.Context, appID int) (ProtonDBRating, error) {
	url := fmt.Sprintf("%s/%d.json", c.baseURL, appID)
	if c.Explain != nil {
		c.Explain(url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProtonDBRating{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProtonDBRating{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ProtonDBRating{}, fmt.Errorf("proton: fetching ProtonDB rating for appid %d: HTTP %d", appID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProtonDBRating{}, err
	}
	var r struct {
		Tier             string  `json:"tier"`
		BestReportedTier string  `json:"bestReportedTier"`
		TrendingTier     string  `json:"trendingTier"`
		Confidence       string  `json:"confidence"`
		Score            float64 `json:"score"`
		Total            int     `json:"total"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return ProtonDBRating{}, fmt.Errorf("proton: parsing ProtonDB response for appid %d: %w", appID, err)
	}
	return ProtonDBRating{
		Tier:             r.Tier,
		BestReportedTier: r.BestReportedTier,
		TrendingTier:     r.TrendingTier,
		Confidence:       r.Confidence,
		Score:            r.Score,
		Total:            r.Total,
	}, nil
}
