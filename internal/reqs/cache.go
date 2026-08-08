// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
)

// requirementsTTL is §9's "Game requirements" row: 30 d soft, 180 d hard.
var requirementsTTL = cache.TTL{Soft: 30 * 24 * time.Hour, Hard: 180 * 24 * time.Hour}

func requirementsCacheKey(appID int) string {
	return "requirements-" + strconv.Itoa(appID) + ".json"
}

func loadCachedRequirements(appID int) (*Requirements, cache.Freshness, error) {
	data, age, err := cache.ReadFile(requirementsCacheKey(appID))
	if err != nil {
		return nil, 0, err
	}
	var r Requirements
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, 0, fmt.Errorf("reqs: parsing cached requirements for appid %d: %w", appID, err)
	}
	return &r, cache.Classify(age, requirementsTTL), nil
}

func saveCachedRequirements(r *Requirements) error {
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return cache.WriteFile(body, requirementsCacheKey(r.AppID))
}

// FetchOptions controls how FetchRequirements balances freshness against
// network/cache use, mirroring rigfit's global --offline/--stale-ok flags
// (§11).
type FetchOptions struct {
	Offline bool // never make a network call; cache or fail
	StaleOK bool // serve hard-expired cache instead of refusing (§9)
}

// FetchRequirements returns a game's requirements plus how fresh they
// are, applying the full §9 policy for the "Game requirements" data
// class:
//
//   - Fresh cache: served without a network call.
//   - Stale or missing cache (and not --offline): fetched live and
//     re-cached; a live failure falls back to whatever cache exists,
//     stale or not.
//   - Expired cache without --stale-ok (and not --offline): refused, per
//     §9's "Refuse; require refresh or --stale-ok" — this is the one
//     case FetchRequirements returns an error for even though a cached
//     copy technically exists.
//   - --offline: cache only, always labelled Stale per §9 point 4 ("serves
//     cache, labelled stale") even if it happens to still be within its
//     soft TTL, since rigfit can't confirm it's still accurate without a
//     network call.
func (c *SteamClient) FetchRequirements(ctx context.Context, appID int, opts FetchOptions) (*Requirements, cache.Freshness, error) {
	cached, freshness, cacheErr := loadCachedRequirements(appID)
	haveCache := cacheErr == nil

	if opts.Offline {
		if !haveCache {
			return nil, 0, fmt.Errorf("reqs: --offline and no cached requirements for appid %d", appID)
		}
		return cached, cache.Stale, nil
	}

	if haveCache && freshness == cache.Fresh {
		return cached, freshness, nil
	}
	if haveCache && freshness == cache.Expired && !opts.StaleOK {
		return nil, 0, fmt.Errorf("reqs: cached requirements for appid %d expired more than %s ago; pass --stale-ok or refresh online", appID, requirementsTTL.Hard)
	}

	fresh, err := c.AppDetails(ctx, appID)
	if err != nil {
		if haveCache {
			return cached, freshness, nil
		}
		return nil, 0, fmt.Errorf("reqs: fetching requirements for appid %d: %w", appID, err)
	}

	if err := saveCachedRequirements(fresh); err != nil {
		return nil, 0, err
	}
	return fresh, cache.Fresh, nil
}
