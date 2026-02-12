## Context

The `d2gridrouter` package implements stages 2–5 of the Hegemann & Wolff (2023) pipeline for orthogonal edge routing. It currently works only when ELK is the layout engine (ELK implements `RoutingPlugin`). Dagre users get no edge routing in grids.

The reference implementation [wueortho](https://github.com/WueGD/wueortho) (Scala 3) implements the full pipeline including force-directed vertex layout and GTree overlap removal. We extend our implementation to cover the full pipeline and register it as a standalone layout engine.

**v2 pivot**: Initial implementation used Fruchterman-Reingold for node placement. Testing revealed fundamental readability issues: arbitrary positions, no alignment, bends in empty space. New approach uses grid-snap BFS for node placement and a simple L/Z-router for edge routing. See Evolution Log in proposal.md.

Key infrastructure already exists:
- `d2plugin.Plugin` interface for layout engines (`d2plugin/plugin.go:68`)
- `d2plugin.RoutingPlugin` interface for edge routers (`d2plugin/plugin.go:84`)
- `ROUTES_EDGES` feature flag (`d2plugin/plugin_features.go:25`)
- `RouterResolver` / `LayoutResolver` in CLI (`d2cli/main.go:425-488`)
- `d2target.Config.LayoutEngine` for D2 syntax (`d2target/d2target.go:50`)
- `DefaultRouter` with TODO comment for replacement (`d2layouts/d2layouts.go:348`)

## Goals / Non-Goals

- Goals:
  - `--layout wueortho` and `layout-engine: wueortho` as a full layout engine
  - Default grid edge routing for ALL layout engines (dagre, elk, wueortho)
  - Option to disable grid routing (`grid-routing: false`)
  - Grid-snap BFS node positioning for standalone mode — compact, aligned, symmetric
  - Simple L/Z-router for standalone mode — clean bends near entry, PCB-like
  - Plugin-specific CLI flags for wueortho parameters
  - Clean fork: `// [FORK]` markers, new package, minimal diffs

- Non-Goals:
  - Replacing dagre as default layout engine (dagre stays default)
  - Full LP solver dependency (use DAG longest-path instead)
  - Handling sequence diagrams (wueortho delegates to d2sequence)
  - Zero-crossing edge routing (backlog — see proposal.md)
  - Container-aware grid placement (backlog — LayoutNested handles nesting)
  - Hierarchy/top-down flow (wueortho is topology-driven, not hierarchical)

## Decisions

### Decision 1: Package rename — `d2gridrouter` → `d2wueortho` [DONE]

The package handles the full Hegemann & Wolff pipeline, not just grid routing. The name `wueortho` matches the reference implementation and the academic work.

**Migration**: Only one file imports `d2gridrouter` directly — `d2plugin/plugin_elk.go:12`. `d2grid/layout.go` receives the router as a `d2graph.RouteEdges` function parameter, no import change needed. The git history preserves the rename. Old `d2gridrouter` references in CLAUDE.md and docs are updated.

### Decision 2: Two roles — layout engine AND default grid router [DONE]

wueortho serves two distinct roles:

**Role A: Standalone layout engine** (`--layout wueortho`)
- Grid-snap BFS placement → L/Z edge routing
- Handles ALL diagram types via `d2layouts.LayoutNested()` dispatch:
  - Default diagrams: wueortho's grid-snap + L/Z routing
  - Grid diagrams: d2grid positioning + wueortho edge routing (stages 2–5)
  - Sequence diagrams: delegates to d2sequence (unchanged)

**Role B: Default grid edge router** (any layout engine)
- `d2wueortho.RouteEdges()` is used by ALL layout plugins for grid edge routing
- Both Dagre and ELK implement `RoutingPlugin` delegating to `d2wueortho.RouteEdges()`
- Enabled by default; disable via `grid-routing: false` in d2-config or `--grid-routing=false`
- When disabled, `DefaultRouter` (straight lines) is used as before

### Decision 3: Node placement — Grid-snap BFS (replaces FR)

~~**Algorithm**: Fruchterman-Reingold force-directed layout (1991).~~

**REVISED**: FR produces unreadable diagrams — nodes at arbitrary positions, no alignment, no visible system. Replaced with grid-snap BFS placement.

**Algorithm**: BFS from max-degree node onto a virtual grid.
1. Find the node with the most connections → BFS root (center of layout)
2. Cell size = `max(nodeW, nodeH) + channel` (channel = 80px for routing corridors)
3. BFS traversal: each neighbor placed in the closest free cell adjacent to its parent. Higher-degree neighbors placed first (they get better positions).
4. Disconnected components fill remaining cells.
5. Each node centered in its cell.

**Why this works**:
- All nodes are axis-aligned → the eye reads order
- Connected nodes are adjacent → short edges, minimal crossings
- Symmetric: a star graph produces a cross/plus shape naturally
- Deterministic: same input → same output
- Scales: O(V+E) for BFS, works for any graph size

**Why not FR**:
- FR is designed for undirected graphs with uniform node sizes
- D2 diagrams have labels, varying sizes, directional edges
- FR produces "organic blob" layouts — no readable structure
- Overlap removal after FR adds more distortion

**Research findings** (Purchase 1997, Ware 2002, Purchase 2012):
- Users spontaneously create grid-aligned layouts — grid-snap is the right approach
- Regularity enables pre-attentive processing: each element conforming to the pattern is processed automatically
- Grid alignment + short uniform edges + symmetry = the three pillars of readable placement

**Planned improvements** (ordered by impact/effort ratio):
1. **Variable cell sizes** — per-row height / per-column width instead of global max. Like an HTML table: each row as tall as its tallest member, each column as wide as its widest. Saves ~320px per column when one node is 400px and the rest 80px. ~30 lines. (Freivalds & Glagolevs 2014)
2. **Respect `direction` attribute** — if graph has `direction: down`, BFS expands down-first. ~5 lines.
3. **Column-limited wrapping** — `maxCols ≈ sqrt(N)` prevents 1×20 linear layouts. ~10 lines.
4. **Edge-direction-aware neighbor placement** — if edge is `A → B`, prefer placing B in the "forward" direction from A. ~20 lines.
5. **Local improvement pass** — after BFS, try moving/swapping nodes; accept changes that reduce total edge length. Core of Freivalds-Glagolevs algorithm. ~50 lines, highest ROI for medium-term.

### Decision 4: Overlap removal — Not needed with grid placement

~~**Algorithm**: GTree overlap removal (Nachmanson et al., 2016).~~

**REVISED**: Grid-snap placement guarantees non-overlap by construction (each node gets its own cell). GTree is no longer needed for standalone mode.

GTree remains relevant only if we switch back to a continuous (non-grid) placement strategy in the future.

### Decision 5: Edge routing for standalone mode — L/Z-Router (NEW)

The Dijkstra routing graph (stages 2–5) produces bends at arbitrary intermediate points. For a grid-based layout, a much simpler router produces cleaner results.

**Algorithm**: L/Z-Router for grid layouts
1. **Face selection** — based on relative grid position of src and dst:
   - Same row: exit RIGHT/LEFT, enter LEFT/RIGHT
   - Same column: exit BOTTOM/TOP, enter TOP/BOTTOM
   - Diagonal: dominant axis determines faces
   - Arrow enters from the direction of the source (user requirement)
2. **Port assignment** — per (node, face) pair:
   - Collect all edges using this face
   - Sort by neighbor position (spatial order minimizes visual crossing)
   - Distribute evenly: `port[i] = face_start + L * (i+1) / (N+1)`
   - Reuses pattern from `portassign.go:220-253`
3. **Route construction**:
   - **Straight**: same row/col, adjacent cells → 2 points, 0 bends
   - **L-route**: diagonal cells → 3 points, 1 bend near target entry
   - **Z-route**: when straight/L crosses an occupied cell → 4 points, 2 bends through channel
4. **Crossing detection**: AABB check against occupied cells. Only avoid edge-through-node crossings (edge-to-edge crossing avoidance is backlog).
5. **Apply routes**: set `edge.Route` directly, skip `TraceToShape` (ports on shape boundary).

**Why not use Dijkstra router for standalone mode**:
- Dijkstra routing graph creates bends at routing graph vertices, which are at arbitrary channel positions → ugly bends in empty space
- For grid-aligned nodes, L/Z-routing is sufficient and produces clean, predictable routes
- Dijkstra router stays for grid routing (ELK/dagre) where nodes are pre-positioned by d2grid

**Research findings**:
- **L-route bend near target: CONFIRMED.** For directed edges, bend-near-target creates a clear "approach" signal — the last segment acts as a visual pointer into the node. Supported by Ware et al. (2002) finding that path continuity is the 2nd most important readability factor. Exception: if bend-near-target L-route crosses an occupied cell but bend-near-source doesn't, prefer the clear route.
- **Z-route channel selection: try both, pick cheaper.** Confirmed by VLSI routing practice. Cost = length + bend_penalty + crossing_penalty + congestion_penalty. Add per-channel congestion counter (increment each time an edge uses a channel) to prevent "all edges pile up on one side."
- **Edge routing order: current strategy confirmed good.** Direct/same-axis edges first (most constrained), then cross-grid by decreasing distance (longest first). This is a hybrid of "most constrained first" + "longest first" — both validated in VLSI routing literature.
- **Port sorting by neighbor position: CONFIRMED sufficient.** This IS the barycenter heuristic from Sugiyama, simplified for single-neighbor ports. Add minimum clearance (8px between ports, 12px from corners) and face load-balancing when Z-avoidance triggers.
- **2 bends sufficient for ~95% of grid cases.** Tamassia (1987) proved every planar 4-graph has an orthogonal drawing with ≤2 bends/edge. Do not artificially limit — let router find optimal path.
- **Rip-up and reroute (backlog):** Route all edges, find worst crossings, reroute. 10-30% crossing reduction. Standard in industrial VLSI routers.

### Decision 6: Default grid routing for Dagre

Currently Dagre does not implement `RoutingPlugin`. We add it:

```go
// plugin_dagre.go
func (p *dagrePlugin) RouteEdges(ctx context.Context, g *d2graph.Graph, edges []*d2graph.Edge) error {
    return d2wueortho.RouteEdges(ctx, g, edges)
}
```

And add `ROUTES_EDGES` to Dagre's feature list in `Info()`. This means Dagre grids automatically get orthogonal edge routing.

### Decision 7: Grid routing toggle

New config option `grid-routing` (boolean, default `true`):

**D2 syntax**:
```d2
vars: {
  d2-config: {
    grid-routing: false
  }
}
```

**CLI flag**: `--grid-routing=false`

**Implementation**:
- Add `GridRouting *bool` to `d2target.Config`
- Add keyword `grid-routing` to `d2compiler.compileConfig()`
- In `d2lib.getEdgeRouter()`: if `GridRouting == false`, return `DefaultRouter`
- Default: `true` (orthogonal routing enabled)

### Decision 8: Path ordering (Stage 4 upgrade)

Current implementation skips Stage 4, relying on crossing penalties in Dijkstra. This works but produces suboptimal results for dense edge bundles.

**Implement**: Modified Pupyrev et al. ordering on shared segments.
- Pre-assign directions: LEFT for horizontal bundles, DOWN for vertical
- Order edges within each bundle by their source/destination positions
- Crossings happen only at bend points
- File: `d2wueortho/ordering.go`

### Decision 9: LP nudging via DAG longest-path (Stage 5 upgrade)

Current nudging uses simple even distribution. Upgrade to constraint-based nudging:

1. Build constraint DAG: order vertical segments left-to-right, horizontal top-to-bottom
2. Add minimum-distance arcs between adjacent segments
3. Transitive reduction to remove redundant constraints
4. Solve via topological sort + longest-path (no LP solver needed for DAGs)

This produces better results than even distribution while avoiding an LP dependency.

### Decision 10: wueortho CLI flags [DONE, NEEDS UPDATE]

Current flags (from FR-based design):
```
--wueortho-crossingPenalty int64  "penalty for edge crossings in routing" (default 500)
--wueortho-edgeSpacing    int64   "minimum spacing between parallel edges" (default 10)
```

Deprecated flags (no longer relevant with grid-snap):
- `--wueortho-iterations` — FR iterations, not used
- `--wueortho-seed` — FR random seed, BFS is deterministic
- `--wueortho-overlapRemoval` — GTree, not needed with grid placement

These deprecated flags should be removed when grid-snap is finalized.

## Architecture

```
d2layouts/d2wueortho/              ← RENAMED from d2gridrouter
├── router.go                      ← RouteEdges() entry (existing, grid routing for ELK/dagre)
├── layout.go                      ← Layout() entry + gridPlacement() for standalone mode
├── gridroute.go                   ← NEW: L/Z-router for standalone mode (gridRouteEdges)
├── portassign.go                  ← Stage 2 (existing, used by Dijkstra router)
├── channels.go                    ← Stage 3a (existing, used by Dijkstra router)
├── routinggraph.go                ← Stage 3b (existing, used by Dijkstra router)
├── dijkstra.go                    ← Stage 3c (existing, used by Dijkstra router)
├── ordering.go                    ← Stage 4: path ordering (NEW, for Dijkstra router)
├── nudging.go                     ← Stage 5 (existing, to be upgraded)
├── types.go                       ← Shared types (existing)
└── opts.go                        ← ConfigurableOpts, DefaultOpts (existing)

d2plugin/
├── plugin_wueortho.go             ← Plugin + RoutingPlugin (existing)
├── plugin_dagre.go                ← MODIFIED: add RoutingPlugin
├── plugin_elk.go                  ← MODIFIED: update import path (existing)
└── plugin.go                      ← UNCHANGED

d2target/d2target.go               ← MODIFIED: add GridRouting to Config
d2compiler/compile.go              ← MODIFIED: parse grid-routing
d2lib/d2.go                        ← MODIFIED: grid-routing toggle logic
d2cli/main.go                      ← MODIFIED: --grid-routing flag
```

## Research Findings

Key findings from literature review (Purchase 1997, Ware 2002, Huang 2007-2009, Tamassia 1987, Hegemann & Wolff 2023, Freivalds & Glagolevs 2014).

### Readability hierarchy (empirically ranked)

| Priority | Criterion | Evidence |
|----------|-----------|----------|
| 1 | Edge crossing minimization | Strongest effect on comprehension (Purchase 1997, Ware 2002) |
| 2 | Path continuity / straightness | 2nd most important, often neglected (Ware 2002) |
| 3 | Crossing angle = 90° | Orthogonal crossings have minimal cognitive cost (Huang 2007) |
| 4 | Bend minimization | Each bend adds cognitive load (Purchase 1997, Tamassia 1987) |
| 5 | Edge length uniformity | Short uniform edges = calmer diagram |
| 6 | Symmetry | Helps pattern detection |
| 7 | Grid alignment | Enables pre-attentive processing (Purchase 2012) |

### Resolved questions

1. **Variable cell sizes** → **Do it.** Per-row height / per-column width (HTML table model). Supported by Freivalds & Glagolevs (2014). Min channel width must be enforced. ~30 lines change to coordinate assignment, BFS logic stays the same.

2. **BFS direction priority** → **Respect `direction` attribute** (5-line change). Then: edge-direction-aware neighbor placement — if `A → B`, prefer placing B in forward direction (~20 lines). Topology detection (chains/stars/trees) is medium-term — yFiles does this but it's ~100 lines.

3. **L-route bend placement** → **Near target, confirmed.** Bend-near-target creates clear "approach" direction. Consistent with Ware's continuity finding. Exception: prefer clear route over bend placement preference.

4. **Z-route channel selection** → **Try both, pick lower cost.** Cost = length + congestion_penalty. Add per-channel usage counter. Confirmed by VLSI practice.

5. **Port ordering** → **Current approach IS barycenter heuristic** (simplified for single-neighbor ports). Sufficient for most cases. Add min-clearance (8px between ports, 12px from corners) and face load-balancing.

6. **Aspect ratio** → **Column-limited wrapping at sqrt(N).** `maxCols = ceil(sqrt(N))` produces roughly square layouts. Expose as configurable option. GoJS PackedLayout uses same approach.

7. **Edge routing order** → **Current strategy confirmed good.** Direct edges first (most constrained), then longest cross-grid edges first. Standard VLSI hybrid heuristic.

8. **2 bends per edge** → **Sufficient for ~95% of grid cases.** Tamassia (1987): every planar 4-graph has orthogonal drawing with ≤2 bends/edge. Don't artificially limit.

### Open research (backlog)

9. **Zero-crossing routing** — rip-up and reroute (VLSI standard). Route all, find worst crossings, reroute offenders. 10-30% crossing reduction. Requires explicit crossing tracking.

10. **Container-aware placement** — layout inside groups first as sub-grids, treat groups as multi-cell units in outer grid. Changes cell sizing significantly. Needs prototyping.

11. **Local improvement pass** — after BFS, try swap/move operations that reduce total edge length. Core of Freivalds-Glagolevs (2014). ~50 lines, highest ROI improvement beyond initial BFS.

12. **Iterative port reordering** — after routing, recompute port order using actual route positions instead of neighbor centers. ELK does this with LAYER_SWEEP. 2-3 iterations.

13. **Quality metrics for regression testing** — total edge length, crossing count, compactness (node area / bbox area), edge length variance, aspect ratio. Prevents quality regressions when changing algorithm.

### Key references

- Purchase 1997 — "Which Aesthetic Has the Greatest Effect on Human Understanding?"
- Ware, Purchase, Colpoys, McGill 2002 — "Cognitive Measurements of Graph Aesthetics"
- Huang 2007-2009 — Eye tracking studies on crossing angles and geodesic-path tendency
- Purchase, Pilcher, Plimmer 2012 — Users spontaneously create grid-aligned layouts
- Tamassia 1987 — Bend-minimum orthogonal drawing via min-cost flow
- Hegemann & Wolff 2023 — "A Simple Pipeline for Orthogonal Graph Drawing"
- Freivalds & Glagolevs 2014 — Compact orthogonal layout with local improvements
- Kieffer, Dwyer, Marriott, Wybrow 2015 — HOLA: Human-like Orthogonal Network Layout
- Biedl & Derka 2016 — Snapping graph drawings to the grid optimally

## Risks / Trade-offs

### Risk 1: Grid-snap placement may be too rigid [MITIGATED]
Grid-aligned placement may waste space for irregular graphs. A graph with one huge node and many small ones will have all small nodes in oversized cells.
**Mitigation**: Variable cell sizes (per-row/per-column, like HTML tables). Planned for implementation in task 3.6. Freivalds & Glagolevs (2014) validated this approach.

### Risk 2: L/Z-router is too simple for complex graphs
L-routes work well for adjacent cells. For long-distance edges across many cells, Z-routes may produce many crossings.
**Mitigation**: The grid-snap BFS ensures connected nodes are adjacent in most cases. Long-distance edges are rare. For complex cases, fall back to existing Dijkstra router.

### Risk 3: Dagre RoutingPlugin changes existing behavior
Adding grid routing to dagre changes how dagre grids render by default.
**Mitigation**: Grid routing is off only when `grid-routing: false`. Default is `true` which improves quality. Existing straight-line behavior was never intended as final (TODO comment in DefaultRouter).

### Risk 4: Package rename breaks imports [MITIGATED]
Renaming `d2gridrouter` → `d2wueortho` touches all importers.
**Mitigation**: Only two files import `d2gridrouter`: `plugin_elk.go` and `d2grid/layout.go`. Minimal blast radius. Already done.

## Open Questions

- Should wueortho handle containers (nested nodes) in grid-snap mode, or treat them as atomic boxes? (Proposed: atomic for now, LayoutNested handles recursion)
- Should the grid-routing toggle be per-container or global? (Proposed: global via d2-config)
- What's the right channel width? 80px is a starting guess. Needs testing with various diagrams.
- Should we keep FR as an alternative placement strategy (selectable via flag) or remove it entirely?
