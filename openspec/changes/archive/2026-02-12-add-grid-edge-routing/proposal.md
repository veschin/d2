# Change: Add proper edge routing for grid diagrams and extend ELK configuration

## Why
Grid diagrams in D2 currently render all edges as simple straight lines (center-to-center) because the layout engines (ELK/Dagre) are completely bypassed for edge routing within grids. This makes complex grid diagrams with many connections look ugly — edges overlap, cross through shapes, and have no intelligent pathfinding. Additionally, users with large diagrams lack sufficient layout configuration options to tune spacing, crossing minimization, and edge routing strategies.

## What Changed (Iteration History)

### Iteration 1: ELK-based post-routing (FAILED)
Created `d2elkedgerouter` package that builds an ELK graph with pre-positioned nodes and extracts edge routes. **Failed** because ELK's layered (Sugiyama) algorithm fundamentally cannot preserve fixed node positions — it reassigns nodes to layers and repositions them. Attempts to use INTERACTIVE strategies and coordinate translation (interpolation, nearest-node offset) all produced broken results: edges crossing through nodes, diagonal artifacts in orthogonal routes.

**Root cause**: ELK is a NODE LAYOUT engine that routes edges as part of layout. It's not a standalone edge router. In a grid, edges can go in any direction (left, right, up, down), but in a layered layout all edges must go in one direction (e.g., top to bottom). This is a fundamental architectural mismatch.

### Iteration 2: Orthogonal Grid Router based on Hegemann & Wolff (2023) (CURRENT)
Replace ELK-based routing with a pure Go orthogonal edge router based on the academic paper:

> Tim Hegemann, Alexander Wolff. "A Simple Pipeline for Orthogonal Graph Drawing."
> GD 2023, LNCS 14466, pp. 170-186, Springer. arXiv:2309.01671.

This paper describes a 5-stage modular pipeline. We use stages 2-5 (port assignment, routing, ordering, nudging) since grid layout handles stage 1 (node positioning).

## What Changes
- Replace `d2elkedgerouter` with `d2gridrouter` — pure Go orthogonal router based on Hegemann & Wolff (2023)
- Implement: port assignment, visibility-graph-based routing, path ordering, LP-based nudging
- Keep the existing `RoutingPlugin` integration and `edgeRouter` parameter in `d2grid.Layout()`
- Keep the extended ELK `ConfigurableOpts` and CLI flags (done in iteration 1, works correctly)
- Consider: insert routing rows/columns in grids when dimensions allow, for more routing space

## Impact
- Affected specs: grid-layout, elk-configuration
- Affected code:
  - `d2layouts/d2gridrouter/` (NEW package — replaces d2elkedgerouter)
  - `d2layouts/d2elkedgerouter/` (DEPRECATED — to be removed or kept as fallback)
  - `d2layouts/d2grid/layout.go` (already modified: edgeRouter parameter)
  - `d2layouts/d2layouts.go` (already modified: passes edgeRouter to grid layout)
  - `d2layouts/d2elklayout/layout.go` (DONE: expanded ConfigurableOpts)
  - `d2plugin/plugin_elk.go` (DONE: RoutingPlugin, ROUTES_EDGES, CLI flags)
- Backward compatible: orthogonal routing when available, falls back to straight lines
- Fork-specific: all changes marked with `// [FORK]` comments
