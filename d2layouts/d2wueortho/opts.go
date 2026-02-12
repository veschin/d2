// [FORK] This file is added by the fork for the wueortho layout engine.
// Configurable options for the wueortho pipeline.

package d2wueortho

// ConfigurableOpts contains tunable parameters for the wueortho layout engine.
type ConfigurableOpts struct {
	// CrossingPenalty is the weight added to routing graph edges that cross already-routed edges.
	// Higher values produce fewer crossings but potentially longer edge routes.
	CrossingPenalty int64 `json:"crossingPenalty"`
	// EdgeSpacing is the minimum pixel spacing between parallel edges in shared corridors.
	EdgeSpacing int64 `json:"edgeSpacing"`
}

// DefaultOpts returns the default configuration for the wueortho layout engine.
var DefaultOpts = ConfigurableOpts{
	CrossingPenalty: 500,
	EdgeSpacing:     10,
}
