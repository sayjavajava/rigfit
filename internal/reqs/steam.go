// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	appDetailsURL = "https://store.steampowered.com/api/appdetails"

	// storeSearchURL resolves a game name to an appid. rigfit-BUILD-SPEC.md
	// §6.1 says name resolution goes through GetAppList, but as of this
	// writing api.steampowered.com/ISteamApps/GetAppList (and its
	// IStoreService/GetAppList successor) either 404s or requires a Steam
	// Web API key — verified live against both endpoints. storesearch is
	// the same keyless, public endpoint the Steam store's own search box
	// uses, and is arguably a better fit for rigfit's zero-config design
	// than caching and locally matching against Steam's entire ~150k-app
	// catalogue would have been.
	storeSearchURL = "https://store.steampowered.com/api/storesearch/"
)

// NormalizerVersion identifies the current parsing logic's output shape
// (§9 point 5: "Cache keyed by normalizer_version; bumping it invalidates
// affected entries"). Bump this when ParseTier's behaviour changes in a
// way that should invalidate previously-cached "auto" records.
const NormalizerVersion = "1.0.0"

// §6.1: rate limit ≈ 200 req / 5 min / IP.
const (
	rateLimitCapacity = 200.0
	rateLimitWindow   = 5 * time.Minute
)

const userAgent = "rigfit (+https://github.com/sayjavajava/rigfit)"

// SteamClient talks to the (undocumented, keyless) Steam store endpoints
// rigfit depends on, honouring the §6.1 rate limit and §2's "no network
// calls other than: Steam API, ProtonDB, AreWeAntiCheatYet" constraint.
type SteamClient struct {
	httpClient *http.Client
	limiter    *tokenBucket

	// appDetailsURL/storeSearchURL are overridden in tests to point at an
	// httptest server instead of the real Steam endpoints.
	appDetailsURL  string
	storeSearchURL string

	// Explain, when non-nil, is called with the full URL immediately
	// before every network request — §11's --explain flag ("print every
	// network call before making it").
	Explain func(url string)
}

// NewSteamClient constructs a SteamClient with the §6.1 rate limit
// applied to every request it makes.
func NewSteamClient() *SteamClient {
	return &SteamClient{
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		limiter:        newTokenBucket(rateLimitCapacity, rateLimitCapacity/rateLimitWindow.Seconds()),
		appDetailsURL:  appDetailsURL,
		storeSearchURL: storeSearchURL,
	}
}

// SearchApp resolves a game name to a Steam appid via the store's public
// search endpoint, preferring an exact case-insensitive name match and
// otherwise falling back to Steam's own top-ranked result — the same
// "closest match" behaviour a user typing into the store search box
// would see, not a rigfit-invented fuzzy match.
func (c *SteamClient) SearchApp(ctx context.Context, name string) (appID int, matchedName string, err error) {
	q := url.Values{"term": {name}, "l": {"english"}, "cc": {"us"}}
	body, err := c.get(ctx, c.storeSearchURL+"?"+q.Encode())
	if err != nil {
		return 0, "", err
	}

	var result struct {
		Total int `json:"total"`
		Items []struct {
			Type string `json:"type"`
			Name string `json:"name"`
			ID   int    `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, "", fmt.Errorf("reqs: parsing storesearch response: %w", err)
	}
	if result.Total == 0 {
		return 0, "", fmt.Errorf("reqs: no Steam listing found for %q", name)
	}

	wantName := strings.ToLower(strings.TrimSpace(name))
	var firstApp *struct {
		Type string `json:"type"`
		Name string `json:"name"`
		ID   int    `json:"id"`
	}
	for i := range result.Items {
		item := &result.Items[i]
		if item.Type != "app" {
			continue
		}
		if firstApp == nil {
			firstApp = item
		}
		if strings.ToLower(strings.TrimSpace(item.Name)) == wantName {
			return item.ID, item.Name, nil
		}
	}
	if firstApp != nil {
		return firstApp.ID, firstApp.Name, nil
	}
	return 0, "", fmt.Errorf("reqs: no Steam app found for %q", name)
}

// AppDetails fetches and parses a game's requirements fresh from Steam
// (no cache lookup — see cache.go's FetchRequirements for the
// cache/TTL-aware entry point callers should normally use).
func (c *SteamClient) AppDetails(ctx context.Context, appID int) (*Requirements, error) {
	q := url.Values{"appids": {strconv.Itoa(appID)}, "cc": {"us"}, "l": {"en"}}
	body, err := c.get(ctx, c.appDetailsURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}

	var envelope map[string]struct {
		Success bool `json:"success"`
		Data    *struct {
			Name              string          `json:"name"`
			Type              string          `json:"type"`
			Platforms         Platforms       `json:"platforms"`
			PCRequirements    json.RawMessage `json:"pc_requirements"`
			LinuxRequirements json.RawMessage `json:"linux_requirements"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("reqs: parsing appdetails response for appid %d: %w", appID, err)
	}
	entry, ok := envelope[strconv.Itoa(appID)]
	if !ok || !entry.Success || entry.Data == nil {
		return nil, fmt.Errorf("reqs: appid %d not found on Steam", appID)
	}

	pcBlob, err := parseRequirementsBlob(entry.Data.PCRequirements)
	if err != nil {
		return nil, fmt.Errorf("reqs: parsing pc_requirements for appid %d: %w", appID, err)
	}
	hash := sha256.Sum256([]byte(pcBlob.Minimum + "\x00" + pcBlob.Recommended))

	req := &Requirements{
		AppID:             appID,
		Name:              entry.Data.Name,
		Source:            "steam",
		FetchedAt:         time.Now().UTC(),
		RawHash:           "sha256:" + hex.EncodeToString(hash[:]),
		NormalizerVersion: NormalizerVersion,
		ReviewStatus:      ReviewAuto,
		Platforms:         entry.Data.Platforms,
		Tiers: Tiers{
			Minimum:     ParseTier(pcBlob.Minimum),
			Recommended: ParseTier(pcBlob.Recommended),
		},
	}

	if entry.Data.Platforms.Linux {
		linuxBlob, err := parseRequirementsBlob(entry.Data.LinuxRequirements)
		if err != nil {
			return nil, fmt.Errorf("reqs: parsing linux_requirements for appid %d: %w", appID, err)
		}
		req.LinuxTiers = &Tiers{
			Minimum:     ParseTier(linuxBlob.Minimum),
			Recommended: ParseTier(linuxBlob.Recommended),
		}
	}

	return req, nil
}

// requirementsBlob is the {minimum, recommended} shape shared by
// pc_requirements, linux_requirements, and mac_requirements.
type requirementsBlob struct {
	Minimum     string `json:"minimum"`
	Recommended string `json:"recommended"`
}

// parseRequirementsBlob decodes one of the *_requirements fields. Steam
// returns an object when requirements text exists, but sometimes an empty
// JSON array `[]` instead of `{}` (or omits the field) when it doesn't —
// verified live. Anything that isn't a `{...}` object is treated as "no
// data" rather than an error; a malformed object is still a real error.
func parseRequirementsBlob(raw json.RawMessage) (requirementsBlob, error) {
	var blob requirementsBlob
	if len(raw) == 0 || raw[0] != '{' {
		return blob, nil
	}
	err := json.Unmarshal(raw, &blob)
	return blob, err
}

// get performs a rate-limited GET, retrying on 429 with exponential
// backoff (honouring Retry-After when present) per §6.1.
func (c *SteamClient) get(ctx context.Context, requestURL string) ([]byte, error) {
	if c.Explain != nil {
		c.Explain(requestURL)
	}

	const maxRetries = 5
	backoff := time.Second

	for attempt := 0; ; attempt++ {
		if err := c.limiter.wait(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("reqs: requesting %s: %w", requestURL, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt >= maxRetries {
				return nil, fmt.Errorf("reqs: %s: rate limited after %d retries", requestURL, maxRetries)
			}
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
			backoff *= 2
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reqs: reading response from %s: %w", requestURL, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("reqs: %s: HTTP %d", requestURL, resp.StatusCode)
		}
		return body, nil
	}
}
