// [FORK] This file is added by the fork for the wueortho layout engine.
// Registers wueortho as a bundled layout plugin alongside dagre and ELK.

//go:build !nowueortho

package d2plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2wueortho"
	"oss.terrastruct.com/util-go/xmain"
)

var WueorthoPlugin = wueorthoPlugin{}

func init() {
	plugins = append(plugins, &WueorthoPlugin)
}

type wueorthoPlugin struct {
	mu   sync.Mutex
	opts *d2wueortho.ConfigurableOpts
}

func (p *wueorthoPlugin) Flags(context.Context) ([]PluginSpecificFlag, error) {
	return []PluginSpecificFlag{
		{
			Name:    "wueortho-crossingPenalty",
			Type:    "int64",
			Default: int64(d2wueortho.DefaultOpts.CrossingPenalty),
			Usage:   "penalty for edge crossings in routing (higher = fewer crossings)",
			Tag:     "crossingPenalty",
		},
		{
			Name:    "wueortho-edgeSpacing",
			Type:    "int64",
			Default: int64(d2wueortho.DefaultOpts.EdgeSpacing),
			Usage:   "minimum pixel spacing between parallel edges",
			Tag:     "edgeSpacing",
		},
	}, nil
}

func (p *wueorthoPlugin) HydrateOpts(opts []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if opts != nil {
		var wueorthoOpts d2wueortho.ConfigurableOpts
		err := json.Unmarshal(opts, &wueorthoOpts)
		if err != nil {
			return xmain.UsageErrorf("non-wueortho layout options given for wueortho")
		}
		p.opts = &wueorthoOpts
	}
	return nil
}

func (p *wueorthoPlugin) Info(ctx context.Context) (*PluginInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	opts := xmain.NewOpts(nil, nil)
	flags, err := p.Flags(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range flags {
		f.AddToOpts(opts)
	}

	return &PluginInfo{
		Name: "wueortho",
		Type: "bundled",
		Features: []PluginFeature{
			ROUTES_EDGES, // [FORK] Enable edge routing for grids
		},
		ShortHelp: "Orthogonal graph drawing engine based on Hegemann & Wolff (2023).",
		LongHelp: fmt.Sprintf(`wueortho is an orthogonal graph drawing engine that positions nodes
using force-directed placement and routes edges with horizontal/vertical
segments only. Based on "A Simple Pipeline for Orthogonal Graph Drawing"
(Hegemann & Wolff, GD 2023, arXiv:2309.01671).

Reference implementation: github.com/WueGD/wueortho (Scala 3).

Flags:
%s
`, opts.Defaults()),
	}, nil
}

func (p *wueorthoPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	p.mu.Lock()
	var optsCopy d2wueortho.ConfigurableOpts
	if p.opts != nil {
		optsCopy = *p.opts
	} else {
		optsCopy = d2wueortho.DefaultOpts
	}
	p.mu.Unlock()
	return d2wueortho.Layout(ctx, g, &optsCopy)
}

// [FORK] RouteEdges implements RoutingPlugin for orthogonal edge routing
// on pre-positioned graphs (e.g., after grid layout).
func (p *wueorthoPlugin) RouteEdges(ctx context.Context, g *d2graph.Graph, edges []*d2graph.Edge) error {
	return d2wueortho.RouteEdges(ctx, g, edges)
}

func (p *wueorthoPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
