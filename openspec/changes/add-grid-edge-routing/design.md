## Context

D2's grid layout (`grid-rows`/`grid-columns`) positions elements in a grid pattern but routes all edges as simple straight lines between cell centers (`d2layouts/d2grid/layout.go:145-162`). The `DefaultRouter` has the same limitation with a `// TODO replace simple straight line edge routing` comment (`d2layouts/d2layouts.go:349`).

The codebase already has infrastructure for separate edge routing:
- `d2graph.RouteEdges` function type (`d2graph/d2graph.go:155`)
- `d2plugin.RoutingPlugin` interface (`d2plugin/plugin.go:84-87`)
- `ROUTES_EDGES` feature flag (`d2plugin/plugin_features.go:25`)
- `RouterResolver` in CLI that checks for the feature and casts to RoutingPlugin (`d2cli/main.go:450-488`)
- `getEdgeRouter` in d2lib that falls back to DefaultRouter (`d2lib/d2.go:167-178`)

This is a fork. Changes should be minimal, clearly marked with `// [FORK]` comments, and preferably in new files/packages to avoid merge conflicts with upstream.

## Failed Approach: ELK-based Post-Routing

### What was tried
Created `d2elkedgerouter` package that:
1. Builds an ELK graph with grid children at their actual positions
2. Sets all INTERACTIVE strategies (cycleBreaking, layering, crossingMinimization, nodePlacement)
3. Runs ELK layout via Goja JS runtime
4. Extracts edge routes and translates coordinates back to grid space

### Why it failed
**Fundamental architectural mismatch**: ELK's layered (Sugiyama) algorithm assigns nodes to discrete layers and routes edges between layers in one direction. In a grid:
- Edge `A→C` (same row) is impossible in a strictly layered layout — both nodes are in the same layer
- Edge `D→B` (bottom-to-top) goes against the layer direction
- ELK MUST rearrange nodes to satisfy layer constraints, regardless of INTERACTIVE settings

**Coordinate translation attempts** (all failed):
1. **Distance-based interpolation**: Blends src/dst offsets per point → breaks orthogonality, creates diagonal artifacts
2. **Nearest-node offset**: Applies closest node's offset to each point → edges still cross through nodes because ELK routed around ELK-positioned nodes, not grid-positioned nodes
3. **Adding all grid siblings**: Even with all cells as ELK nodes + all INTERACTIVE strategies, ELK still repositions nodes between layers

**Conclusion**: ELK is a node layout engine that routes edges as part of layout. It cannot function as a standalone edge router for arbitrary pre-positioned nodes. A fundamentally different approach is needed.

## Goals / Non-Goals
- Goals:
  - Proper edge routing in grid diagrams (orthogonal paths that avoid nodes)
  - Minimum crossings and bends
  - Clean edge separation (no overlapping edges)
  - All rendering-relevant ELK parameters exposed as CLI flags (DONE)
  - Clean fork: new package for router, minimal diffs in existing files
  - Based on well-established academic algorithm, not ad-hoc
- Non-Goals:
  - Using ELK as edge router (proven to not work for this use case)
  - Changing grid positioning logic (cells stay in their grid positions)
  - Full orthogonal graph layout (we only need edge routing with fixed nodes)

## Decisions

### Decision 1: Orthogonal routing based on Hegemann & Wolff (2023)

**Paper**: Tim Hegemann, Alexander Wolff. "A Simple Pipeline for Orthogonal Graph Drawing." GD 2023, LNCS 14466, pp. 170-186. [arXiv:2309.01671](https://arxiv.org/abs/2309.01671).

**Reference implementation**: [github.com/WueGD/wueortho](https://github.com/WueGD/wueortho) (Scala 3)

**Why this paper**:
- Most recent comprehensive treatment (2023) of orthogonal edge routing with fixed node positions
- Modular 5-stage pipeline — we use stages 2-5, grid layout handles stage 1
- LP-based nudging produces clean, well-spaced edge separation
- Experimentally validated against existing approaches
- Clear algorithmic descriptions suitable for implementation
- No Go implementation exists — we implement from scratch based on the paper

**Alternatives considered**:
- Wybrow, Marriott, Stuckey (2010) — "Orthogonal Connector Routing" (libavoid). Good but older; nudging is simpler. No Go port.
- Lee's Algorithm (1961) — BFS maze routing. Foundational but no multi-edge handling, no nudging.
- Tamassia (1987) — Min-cost flow for bend minimization. Optimal but complex; requires planar embedding.

### Decision 2: Pipeline stages we implement

From the paper's 5-stage pipeline:

| Stage | Paper | Our Use | Notes |
|-------|-------|---------|-------|
| 1. Vertex Layout + Overlap Removal | Force-directed + GTree | **SKIP** | Grid layout already positions nodes |
| 2. Port Assignment | Geometric + Z-avoidance | **IMPLEMENT** | Determine where edges exit/enter cells |
| 3. Routing (routing graph + Dijkstra) | Channels + partial grid + Dijkstra | **IMPLEMENT** | Core: build routing graph, find paths |
| 4. Path Ordering | Modified Pupyrev et al. | **IMPLEMENT** | Order edges on shared segments |
| 5. Nudging | LP-based constraint solving | **IMPLEMENT (simplified)** | Balance inter-edge distances |

Stage 5 (nudging) — we implement "constrained nudging" mode (vertex/port positions fixed, only free segments move). Full nudging (moving boxes) is not needed since grid positions are fixed.

For LP solving in stage 5, we can use a simple approach: since our constraint graph is a DAG, we can solve it with topological-sort-based longest-path instead of a full LP solver. This avoids adding an LP dependency.

### Decision 3: Expand ConfigurableOpts (DONE — kept from iteration 1)
All hardcoded ELK layout options are now configurable via CLI flags. This is independent of the edge routing approach and works correctly.

### Decision 4: Future enhancement — routing rows/columns
When grid dimensions allow (e.g., `grid-rows: 2` without `grid-columns`), insert invisible routing columns between data columns to provide more space for edge routing. Not in initial implementation.

## Algorithm Detail (from paper)

### Stage 2: Port Assignment
For each edge uv:
1. Connect centers of u and v with a straight line
2. Assign ports to the box sides that this line intersects
3. Z-shape avoidance: split each side into 4 quarters. If intersection falls in first/last quarter, reassign to make an L-shape (1 bend) instead of Z-shape (2 bends)
4. Order ports on each side by circular order of neighbor directions
5. Distribute ports evenly along each side

### Stage 3: Routing Graph + Edge Routing
**Channel construction** (O(n log n) sweep-line):
- For each pair of adjacent boxes, find the maximal empty rectangle between them → this is a "channel"
- At most 4n channels (one per side per node)
- Each channel gets a "representative" line segment (center line or port-aligned)

**Routing graph H** (partial grid):
- Vertices: ports + intersection points of horizontal/vertical representatives
- Edges: segments between consecutive vertices along representatives
- Gaps where node boxes occupy space → routes cannot go through nodes

**Edge routing** (modified Dijkstra per edge):
- State: (length, bends, direction)
- Minimize length first, then bends (lexicographic)
- Post-process: ensure every pair of edges crosses at most once

### Stage 4: Path Ordering (modified Pupyrev et al.)
- For shared segments (edge bundles), determine order of parallel paths
- Pre-assign directions: LEFT for horizontal, DOWN for vertical
- Crossings happen only at bend points (no extra bends needed)

### Stage 5: Constrained Nudging (LP / topological sort)
- Build constraint graph: DAG ordering all vertical segments left-to-right
- Arcs between vertically-overlapping adjacent objects
- Transitive reduction to remove redundant constraints
- Maximize distance variables while minimizing bounding box
- For grid case: simplified to DAG longest-path (no LP solver needed)

## Architecture

```
d2layouts/d2gridrouter/           ← NEW package (pure Go, no JS dependency)
├── router.go                     ← RouteEdges() entry point
├── portassign.go                 ← Stage 2: port assignment
├── channels.go                   ← Stage 3a: channel construction (sweep-line)
├── routinggraph.go               ← Stage 3b: routing graph (partial grid)
├── dijkstra.go                   ← Stage 3c: modified Dijkstra with bend minimization
├── ordering.go                   ← Stage 4: path ordering on shared segments
├── nudging.go                    ← Stage 5: constrained nudging (DAG longest-path)
└── types.go                      ← Shared data structures
```

## Risks / Trade-offs

### Risk 1: Implementation complexity
The full Hegemann & Wolff pipeline has 5 stages with non-trivial algorithms (sweep-line, Dijkstra, DAG solving).
**Mitigation**: Implement incrementally. Start with stages 2-3 (ports + routing) for a working MVP, then add stages 4-5 (ordering + nudging) for polish. Each stage is independent and testable.

### Risk 2: Edge quality without LP nudging
Without stage 5, edges may bunch together on shared segments.
**Mitigation**: Even without nudging, the routing graph + Dijkstra approach produces correct routes that avoid nodes. Nudging is polish, not correctness.

### Risk 3: Grid-specific simplifications may miss edge cases
The paper targets general orthogonal layouts; our grid case has regular structure that simplifies some steps but may have unique edge cases.
**Mitigation**: Test with diverse grid configurations (different sizes, edge patterns, labels).

### Risk 4: Coordinate system integration
Grid layout uses absolute coordinates; the router must work in the same space.
**Mitigation**: Route edges after grid positioning but before `gd.shift()`, same as current code. All coordinates are in the grid container's local space.

## Key Files

| File | Role | Status |
|------|------|--------|
| `d2layouts/d2gridrouter/*.go` | **NEW** — Orthogonal grid router | TO DO |
| `d2layouts/d2elkedgerouter/router.go` | **DEPRECATED** — Failed ELK approach | REMOVE |
| `d2layouts/d2grid/layout.go` | Grid Layout — edgeRouter parameter | DONE |
| `d2layouts/d2layouts.go` | LayoutNested — passes edgeRouter | DONE |
| `d2layouts/d2elklayout/layout.go` | ELK ConfigurableOpts | DONE |
| `d2layouts/d2elklayout/js.go` | ELK JS exports (for d2elkedgerouter) | REMOVE with d2elkedgerouter |
| `d2plugin/plugin_elk.go` | RoutingPlugin, CLI flags | DONE (update to use d2gridrouter) |
| `d2plugin/plugin.go` | RoutingPlugin interface | EXISTS |
| `d2lib/d2.go` | getEdgeRouter pipeline | EXISTS |
| `d2cli/main.go` | RouterResolver | EXISTS |

## Open Questions
- Should nudging use actual LP (add dependency like gonum/optimize) or DAG longest-path (simpler, sufficient for grids)?
- Should the grid router also be used as DefaultRouter for non-grid cross-container edges?
- Should we support inserting routing rows/columns in grids when dimensions are partially specified?
