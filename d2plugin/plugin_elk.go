//go:build !noelk

package d2plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2layouts/d2gridrouter"
	"oss.terrastruct.com/util-go/xmain"
)

var ELKPlugin = elkPlugin{}

func init() {
	plugins = append(plugins, &ELKPlugin)
}

type elkPlugin struct {
	opts *d2elklayout.ConfigurableOpts
}

func (p elkPlugin) Flags(context.Context) ([]PluginSpecificFlag, error) {
	return []PluginSpecificFlag{
		{
			Name:    "elk-algorithm",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.Algorithm,
			Usage:   "layout algorithm",
			Tag:     "elk.algorithm",
		},
		{
			Name:    "elk-nodeNodeBetweenLayers",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.NodeSpacing),
			Usage:   "the spacing to be preserved between any pair of nodes of two adjacent layers",
			Tag:     "spacing.nodeNodeBetweenLayers",
		},
		{
			Name:    "elk-padding",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.Padding,
			Usage:   "the padding to be left to a parent element’s border when placing child elements",
			Tag:     "elk.padding",
		},
		{
			Name:    "elk-edgeNodeBetweenLayers",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.EdgeNodeSpacing),
			Usage:   "the spacing to be preserved between nodes and edges that are routed next to the node’s layer",
			Tag:     "spacing.edgeNodeBetweenLayers",
		},
		{
			Name:    "elk-nodeSelfLoop",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.SelfLoopSpacing),
			Usage:   "spacing to be preserved between a node and its self loops",
			Tag:     "elk.spacing.nodeSelfLoop",
		},
		// [FORK] Additional ELK options previously hardcoded
		{
			Name:    "elk-thoroughness",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.Thoroughness),
			Usage:   "how much effort ELK spends on producing a good layout (higher = better but slower)",
			Tag:     "elk.layered.thoroughness",
		},
		{
			Name:    "elk-edgeEdgeBetweenLayers",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.EdgeEdgeBetweenLayersSpacing),
			Usage:   "spacing between edges routed between layers",
			Tag:     "elk.layered.spacing.edgeEdgeBetweenLayers",
		},
		{
			Name:    "elk-edgeNode",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.EdgeNodeAbsoluteSpacing),
			Usage:   "spacing between edges and nodes",
			Tag:     "elk.spacing.edgeNode",
		},
		{
			Name:    "elk-fixedAlignment",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.FixedAlignment,
			Usage:   "node alignment strategy: NONE, LEFTUP, RIGHTUP, LEFTDOWN, RIGHTDOWN, BALANCED",
			Tag:     "elk.layered.nodePlacement.bk.fixedAlignment",
		},
		{
			Name:    "elk-considerModelOrder",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.ConsiderModelOrder,
			Usage:   "model order strategy: NONE, NODES_AND_EDGES, PREFER_EDGES, PREFER_NODES",
			Tag:     "elk.layered.considerModelOrder.strategy",
		},
		{
			Name:    "elk-cycleBreakingStrategy",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.CycleBreakingStrategy,
			Usage:   "cycle breaking: GREEDY, GREEDY_MODEL_ORDER, DEPTH_FIRST, INTERACTIVE",
			Tag:     "elk.layered.cycleBreaking.strategy",
		},
		{
			Name:    "elk-crossingMinimization",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.CrossingMinimizationStrategy,
			Usage:   "crossing minimization: LAYER_SWEEP, INTERACTIVE",
			Tag:     "elk.layered.crossingMinimization.strategy",
		},
		{
			Name:    "elk-nodePlacement",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.NodePlacementStrategy,
			Usage:   "node placement: BRANDES_KOEPF, LINEAR_SEGMENTS, SIMPLE, NETWORK_SIMPLEX",
			Tag:     "elk.layered.nodePlacement.strategy",
		},
		{
			Name:    "elk-edgeRouting",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.EdgeRoutingStrategy,
			Usage:   "edge routing style: ORTHOGONAL, POLYLINE, SPLINES",
			Tag:     "elk.layered.edgeRouting",
		},
	}, nil
}

func (p *elkPlugin) HydrateOpts(opts []byte) error {
	if opts != nil {
		var elkOpts d2elklayout.ConfigurableOpts
		err := json.Unmarshal(opts, &elkOpts)
		if err != nil {
			return xmain.UsageErrorf("non-ELK layout options given for ELK")
		}

		p.opts = &elkOpts
	}
	return nil
}

func (p elkPlugin) Info(ctx context.Context) (*PluginInfo, error) {
	opts := xmain.NewOpts(nil, nil)
	flags, err := p.Flags(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range flags {
		f.AddToOpts(opts)
	}
	return &PluginInfo{
		Name: "elk",
		Type: "bundled",
		Features: []PluginFeature{
			CONTAINER_DIMENSIONS,
			DESCENDANT_EDGES,
			ROUTES_EDGES, // [FORK] Enable edge routing for grids
		},
		ShortHelp: "Eclipse Layout Kernel (ELK) with the Layered algorithm.",
		LongHelp: fmt.Sprintf(`ELK is a layout engine offered by Eclipse.
Originally written in Java, it has been ported to Javascript and cross-compiled into D2.
See https://d2lang.com/tour/elk for more.

Flags correspond to ones found at https://www.eclipse.org/elk/reference.html.

Flags:
%s
`, opts.Defaults()),
	}, nil
}

func (p elkPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return d2elklayout.Layout(ctx, g, p.opts)
}

// [FORK] RouteEdges implements RoutingPlugin for ELK-based edge routing
// on pre-positioned graphs (e.g., after grid layout).
// [FORK] RouteEdges uses the orthogonal grid router based on Hegemann & Wolff (2023).
func (p elkPlugin) RouteEdges(ctx context.Context, g *d2graph.Graph, edges []*d2graph.Edge) error {
	return d2gridrouter.RouteEdges(ctx, g, edges)
}

func (p elkPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
