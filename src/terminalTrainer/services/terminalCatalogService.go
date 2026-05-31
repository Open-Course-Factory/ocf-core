package services

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"soli/formations/src/payment/catalog"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/utils"
)

// terminalCatalogService owns the distribution/size/feature catalog concern
// (Phase 4 composed sessions): the size and feature catalog reads — each with
// its own 60s TTL cache — and the session-options computation that intersects
// those catalogs with a plan and a distribution.
//
// It was carved out of terminalTrainerService to shrink that god object;
// terminalTrainerService embeds a *terminalCatalogService and delegates the
// relevant interface methods to it.
//
// Distribution reads go through the shared terminalProxyClient. The size and
// feature reads issue their own tt-backend GET using the connection settings
// passed in at construction, so the catalog owns the full read+cache path.
type terminalCatalogService struct {
	proxy      *terminalProxyClient
	baseURL    string
	apiVersion string
	adminKey   string

	catalogSizesCache        []dto.TTSize
	catalogSizesCacheTime    time.Time
	catalogSizesMu           sync.RWMutex
	catalogFeaturesCache     []dto.TTFeature
	catalogFeaturesCacheTime time.Time
	catalogFeaturesMu        sync.RWMutex
}

// newTerminalCatalogService returns a catalog service reading from tt-backend
// through the supplied proxy (distributions) and connection settings (sizes,
// features). The proxy is shared with the facade so both reuse the same HTTP
// configuration.
func newTerminalCatalogService(proxy *terminalProxyClient, baseURL, apiVersion, adminKey string) *terminalCatalogService {
	return &terminalCatalogService{
		proxy:      proxy,
		baseURL:    baseURL,
		apiVersion: apiVersion,
		adminKey:   adminKey,
	}
}

// ==========================================
// Composed Session (Phase 4)
// ==========================================

const catalogCacheTTL = 60 * time.Second

// featurePlanMapping maps feature keys to plan predicates.
//
// Persistence is intentionally NOT in this map: the persistent-vs-ephemeral
// choice is surfaced as a persistence_mode radio (in TerminalAdvancedOptions),
// not as a "feature" chip in the SessionComposer. It is gated upstream by
// resolvePersistenceMode reading plan.DataPersistenceEnabled (SSOT).
var featurePlanMapping = map[string]func(*paymentModels.SubscriptionPlan) bool{
	"network": func(p *paymentModels.SubscriptionPlan) bool { return p.NetworkAccessEnabled },
}

// NormalizeSizeKey uppercases and trims a size key for comparison
func NormalizeSizeKey(key string) string {
	return strings.ToUpper(strings.TrimSpace(key))
}

// GetCatalogSizes fetches sizes from tt-backend with a 60s TTL cache.
//
// Each entry is enriched with CPUMcpu from ocf-core's payment catalog so
// the wire stops conflating tt-backend's cpuset integer (raw CPU) with the
// effective budget cost. Custom sizes unknown to ocf-core's catalog land
// with CPUMcpu=0 — the frontend treats that as "unknown, fall back to raw".
func (c *terminalCatalogService) GetCatalogSizes() ([]dto.TTSize, error) {
	c.catalogSizesMu.RLock()
	if c.catalogSizesCache != nil && time.Since(c.catalogSizesCacheTime) < catalogCacheTTL {
		cached := c.catalogSizesCache
		c.catalogSizesMu.RUnlock()
		return cached, nil
	}
	c.catalogSizesMu.RUnlock()

	url := fmt.Sprintf("%s/%s/sizes", c.baseURL, c.apiVersion)
	var sizes []dto.TTSize
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(c.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sizes, opts)
	if err != nil {
		return nil, err
	}

	// Stamp the effective mCPU budget cost from the OCF catalog. Sizes the
	// catalog doesn't know about keep CPUMcpu=0 (sentinel for "unknown").
	for i := range sizes {
		sizes[i].CPUMcpu = catalog.MCPUFor(sizes[i].Key)
	}

	c.catalogSizesMu.Lock()
	c.catalogSizesCache = sizes
	c.catalogSizesCacheTime = time.Now()
	c.catalogSizesMu.Unlock()

	return sizes, nil
}

// GetCatalogFeatures fetches features from tt-backend with a 60s TTL cache
func (c *terminalCatalogService) GetCatalogFeatures() ([]dto.TTFeature, error) {
	c.catalogFeaturesMu.RLock()
	if c.catalogFeaturesCache != nil && time.Since(c.catalogFeaturesCacheTime) < catalogCacheTTL {
		cached := c.catalogFeaturesCache
		c.catalogFeaturesMu.RUnlock()
		return cached, nil
	}
	c.catalogFeaturesMu.RUnlock()

	url := fmt.Sprintf("%s/%s/features", c.baseURL, c.apiVersion)
	var features []dto.TTFeature
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(c.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &features, opts)
	if err != nil {
		return nil, err
	}

	c.catalogFeaturesMu.Lock()
	c.catalogFeaturesCache = features
	c.catalogFeaturesCacheTime = time.Now()
	c.catalogFeaturesMu.Unlock()

	return features, nil
}

// ComputeSessionOptions computes allowed sizes and features given catalogs, a distribution, and a plan.
// Exported for testing.
func ComputeSessionOptions(
	distro dto.TTDistribution,
	allSizes []dto.TTSize,
	allFeatures []dto.TTFeature,
	plan *paymentModels.SubscriptionPlan,
) *dto.SessionOptionsResponse {
	// Build a lookup of size sort orders by normalized key
	sizeSortOrder := make(map[string]int, len(allSizes))
	for _, s := range allSizes {
		sizeSortOrder[NormalizeSizeKey(s.Key)] = s.SortOrder
	}

	// Determine the minimum size sort order for this distribution
	minSortOrder := 0
	if distro.MinSizeKey != "" {
		if so, ok := sizeSortOrder[NormalizeSizeKey(distro.MinSizeKey)]; ok {
			minSortOrder = so
		}
	}

	// Evaluate each size. All catalog sizes are admitted; the budget
	// engine sets RemainingCount per size downstream.
	//
	// We re-stamp CPUMcpu from the OCF catalog here so the field is
	// authoritative regardless of whether the caller passed in sizes that
	// already had it populated (GetCatalogSizes path) or bare TTSize
	// literals (tests / future callers). Sizes the catalog doesn't know
	// about keep CPUMcpu=0.
	allowedSizes := make([]dto.SessionOptionSize, 0, len(allSizes))
	for _, s := range allSizes {
		if s.SortOrder < minSortOrder {
			continue
		}
		s.CPUMcpu = catalog.MCPUFor(s.Key)
		allowedSizes = append(allowedSizes, dto.SessionOptionSize{TTSize: s, Allowed: true})
	}

	// Build a set of the distribution's supported features
	supportedFeatures := make(map[string]bool, len(distro.SupportedFeatures))
	for _, f := range distro.SupportedFeatures {
		supportedFeatures[f] = true
	}

	// Find the minimum sort order among allowed sizes (for min_size_key feature check)
	maxAllowedSortOrder := 0
	for _, s := range allowedSizes {
		if s.Allowed && s.SortOrder > maxAllowedSortOrder {
			maxAllowedSortOrder = s.SortOrder
		}
	}

	// Evaluate each feature
	allowedFeatures := make([]dto.SessionOptionFeature, 0, len(allFeatures))
	for _, f := range allFeatures {
		opt := dto.SessionOptionFeature{
			Key:         f.Key,
			Name:        f.Name,
			Description: f.Description,
			Allowed:     true,
		}

		if !supportedFeatures[f.Key] {
			opt.Allowed = false
			opt.Reason = "not_supported"
		} else if checker, ok := featurePlanMapping[f.Key]; ok && !checker(plan) {
			opt.Allowed = false
			opt.Reason = "plan_disabled"
		} else if f.MinSizeKey != "" {
			// Check if at least one allowed size meets the feature's minimum
			featureMinSortOrder := 0
			if so, ok := sizeSortOrder[NormalizeSizeKey(f.MinSizeKey)]; ok {
				featureMinSortOrder = so
			}
			if maxAllowedSortOrder < featureMinSortOrder {
				opt.Allowed = false
				opt.Reason = "size_too_small"
			}
		}

		allowedFeatures = append(allowedFeatures, opt)
	}

	return &dto.SessionOptionsResponse{
		Distribution:    distro,
		AllowedSizes:    allowedSizes,
		AllowedFeatures: allowedFeatures,
	}
}

// GetSessionOptions validates a distribution and computes plan-intersected options
func (c *terminalCatalogService) GetSessionOptions(plan *paymentModels.SubscriptionPlan, distribution string, backend string) (*dto.SessionOptionsResponse, error) {
	distributions, err := c.proxy.GetDistributions(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to get distributions: %w", err)
	}

	var distro *dto.TTDistribution
	for i := range distributions {
		if distributions[i].Name == distribution || distributions[i].Prefix == distribution {
			distro = &distributions[i]
			break
		}
	}
	if distro == nil {
		return nil, fmt.Errorf("distribution '%s' not found", distribution)
	}

	sizes, err := c.GetCatalogSizes()
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog sizes: %w", err)
	}

	features, err := c.GetCatalogFeatures()
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog features: %w", err)
	}

	return ComputeSessionOptions(*distro, sizes, features, plan), nil
}
