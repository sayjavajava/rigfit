// SPDX-License-Identifier: GPL-3.0-only

// Package proton implements rigfit's Linux/Proton-compatibility layer
// (rigfit-BUILD-SPEC.md §8, §13 M6): the anti-cheat gate, ProtonDB
// ratings, and Wine/graphics-stack detection a Linux user needs when a
// game has no native Linux Steam listing. Evaluation order matters —
// the anti-cheat gate is checked first and overrides everything else.
package proton

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
)

// areWeAntiCheatYetURL is AreWeAntiCheatYet's own maintained dataset —
// verified live (2026-08-08) at this path.
const areWeAntiCheatYetURL = "https://raw.githubusercontent.com/AreWeAntiCheatYet/AreWeAntiCheatYet/master/games.json"

// antiCheatTTL is §9's "Anti-cheat status" row: 3 d soft, 14 d hard.
var antiCheatTTL = cache.TTL{Soft: 3 * 24 * time.Hour, Hard: 14 * 24 * time.Hour}

const antiCheatCacheKey = "anticheat-dataset.json"

type awacyEntry struct {
	Name       string   `json:"name"`
	Native     bool     `json:"native"`
	Status     string   `json:"status"` // "Supported" | "Running" | "Denied" | "Broken" | "Planned"
	AntiCheats []string `json:"anticheats"`
	StoreIDs   struct {
		Steam string `json:"steam"`
	} `json:"storeIds"`
}

// GateStatus is the outcome of checking a game against the
// AreWeAntiCheatYet dataset.
type GateStatus int

const (
	// GateNoKnownAntiCheat means the game isn't in the dataset at all.
	// Most Steam games have no invasive anti-cheat, so this is the common
	// case, not an "unrecognised" failure — the dataset is a positive
	// list of known anti-cheat titles, and absence from it is itself a
	// real (if imperfect) signal, unlike pkg/hwid identity resolution
	// where an unmatched device genuinely means "we don't know".
	GateNoKnownAntiCheat GateStatus = iota
	// GatePlayable means the anti-cheat is confirmed working under Proton
	// (dataset status "Supported" or "Running").
	GatePlayable
	// GateCurrentlyBroken means the game uses anti-cheat that doesn't
	// currently work under Proton (status "Broken" or "Planned") but
	// isn't a permanent vendor refusal — a future patch could fix it.
	GateCurrentlyBroken
	// GateBlocked means the anti-cheat vendor or developer has explicitly
	// refused Linux/Proton support (status "Denied"). This is permanent:
	// §8 point 1's "no upgrade changes this".
	GateBlocked
)

func (s GateStatus) String() string {
	switch s {
	case GatePlayable:
		return "playable"
	case GateCurrentlyBroken:
		return "currently_broken"
	case GateBlocked:
		return "blocked"
	default:
		return "no_known_anticheat"
	}
}

// MarshalJSON renders GateStatus as its String() form, matching
// hwid.Confidence/score.Status's approach.
func (s GateStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// AntiCheatResult is the anti-cheat gate's verdict for one game.
type AntiCheatResult struct {
	Status       GateStatus `json:"status"`
	AntiCheats   []string   `json:"anticheats,omitempty"`
	SourceStatus string     `json:"source_status,omitempty"` // raw AreWeAntiCheatYet status, e.g. "Supported"
	// Available is false when the dataset couldn't be checked at all
	// (offline with no fresh cache, or a fetch failure) — distinct from
	// Status == GateNoKnownAntiCheat, which means the check succeeded and
	// found nothing. §9: never presented as a stale/guessed answer.
	Available bool `json:"available"`
}

// Blocks reports whether this result must stop evaluation before any
// spec comparison (§8 point 1).
func (r AntiCheatResult) Blocks() bool {
	return r.Status == GateBlocked || r.Status == GateCurrentlyBroken
}

// AntiCheatClient checks games against the AreWeAntiCheatYet dataset.
type AntiCheatClient struct {
	httpClient *http.Client
	datasetURL string // overridden in tests to point at an httptest server

	// Explain, when non-nil, is called with the full URL immediately
	// before the dataset is fetched — §11's --explain flag.
	Explain func(url string)
}

// NewAntiCheatClient constructs an AntiCheatClient.
func NewAntiCheatClient() *AntiCheatClient {
	return &AntiCheatClient{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		datasetURL: areWeAntiCheatYetURL,
	}
}

// CheckAppID implements §8 point 1 and §9's anti-cheat caching rule.
// Unlike reqs.FetchRequirements, stale data is never served here even
// with an equivalent of --stale-ok: past the 3-day soft TTL, CheckAppID
// always either fetches live or reports Available: false. Presenting an
// outdated anti-cheat-compatibility answer as current is a worse outcome
// than admitting it's unknown (§9: "Never served stale").
func (c *AntiCheatClient) CheckAppID(ctx context.Context, appID int, offline bool) (AntiCheatResult, error) {
	raw, age, cacheErr := cache.ReadFile(antiCheatCacheKey)
	fresh := cacheErr == nil && cache.Classify(age, antiCheatTTL) == cache.Fresh

	if !fresh {
		if offline {
			return AntiCheatResult{Available: false}, nil
		}
		fetched, err := c.fetchDataset(ctx)
		if err != nil {
			// §2A: "degrade gracefully... do not guess" — a fetch failure
			// is not a fatal error for the overall verdict, just an
			// unavailable anti-cheat check.
			return AntiCheatResult{Available: false}, nil
		}
		raw = fetched
		if err := cache.WriteFile(raw, antiCheatCacheKey); err != nil {
			return AntiCheatResult{}, err
		}
	}

	var entries []awacyEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return AntiCheatResult{Available: false}, nil
	}

	appIDStr := strconv.Itoa(appID)
	for _, e := range entries {
		if e.StoreIDs.Steam != appIDStr {
			continue
		}
		return AntiCheatResult{
			Status:       classifyAWACYStatus(e.Status),
			AntiCheats:   e.AntiCheats,
			SourceStatus: e.Status,
			Available:    true,
		}, nil
	}
	return AntiCheatResult{Status: GateNoKnownAntiCheat, Available: true}, nil
}

func classifyAWACYStatus(status string) GateStatus {
	switch status {
	case "Supported", "Running":
		return GatePlayable
	case "Denied":
		return GateBlocked
	case "Broken", "Planned":
		return GateCurrentlyBroken
	default:
		return GateNoKnownAntiCheat
	}
}

func (c *AntiCheatClient) fetchDataset(ctx context.Context) ([]byte, error) {
	if c.Explain != nil {
		c.Explain(c.datasetURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.datasetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proton: fetching anti-cheat dataset: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
