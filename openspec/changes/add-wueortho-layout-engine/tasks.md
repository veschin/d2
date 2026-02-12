## 1. Rename `d2gridrouter` → `d2wueortho` [DONE]
- [x] 1.1 Rename directory `d2layouts/d2gridrouter/` → `d2layouts/d2wueortho/`
- [x] 1.2 Update `package` declaration in all files to `d2wueortho`
- [x] 1.3 Update import in `d2plugin/plugin_elk.go` to `d2layouts/d2wueortho`
- [x] 1.4 Verify `d2layouts/d2grid/layout.go` does NOT directly import d2gridrouter (it receives edgeRouter as function parameter — no changes needed)
- [x] 1.5 Update `[FORK]` comments and `router.go` header to reference wueortho
- [x] 1.6 Update `CLAUDE.md` fork packages section
- [x] 1.7 Verify `go build ./...` passes after rename

## 2. Register wueortho as a layout plugin [DONE]
- [x] 2.1 Create `d2layouts/d2wueortho/opts.go` — `ConfigurableOpts`, `DefaultOpts`
- [x] 2.2 Create `d2plugin/plugin_wueortho.go` with build tag `!nowueortho`
- [x] 2.3 Verify `--layout wueortho` is recognized by CLI
- [x] 2.4 Verify `layout-engine: wueortho` in D2 syntax works
- [x] 2.5 Verify wueortho appears in `d2 layout` listing

## 3. Grid-snap BFS node placement [DONE]
_Replaces FR + GTree. See design.md Decision 3._
- [x] 3.1 Create `layout.go` with `Layout()` entry point
- [x] 3.2 Implement `gridPlacement()` — BFS from max-degree node onto virtual grid
- [x] 3.3 Implement `findBestCell()` — find free cell near parent
- [x] 3.4 Implement `positionLabels()` — set InsideMiddleCenter/OutsideTopCenter
- [x] 3.5 Refactor `gridPlacement` to return placement data (map + cell dimensions) for the router
- [x] 3.6 Variable cell sizes — per-row height / per-column width instead of global max
- [x] 3.7 Respect `direction` attribute in BFS expansion order
- [x] 3.8 Edge-direction-aware neighbor placement
- [x] 3.9 Aspect ratio control — column-limited wrapping at sqrt(N)
- [x] 3.10 Local improvement pass — swap/move optimization after BFS (with crossing penalty)
- [x] 3.11 Test: grid placement with star graph produces symmetric cross/plus shape
- [x] 3.12 Test: grid placement with linear chain produces single row
- [x] 3.13 Test: grid placement with disconnected components places them adjacently
- [x] 3.14 Test: variable cell sizes — large node doesn't inflate all cells

## 4. L/Z-Router for standalone mode [DONE]
_New simple router for grid-aligned nodes. See design.md Decision 5._
_Research resolved: bend near target confirmed, port sorting confirmed, 2 bends sufficient._
- [x] 4.1 Create `d2layouts/d2wueortho/gridroute.go`
- [x] 4.2 Implement face selection — determine exit/entry faces from grid cell positions
  - Same row: RIGHT/LEFT ↔ LEFT/RIGHT
  - Same col: BOTTOM/TOP ↔ TOP/BOTTOM
  - Diagonal: two-pass system:
    - Pass 1: mandatory faces for same-row/col and strictly-dominant diagonals
    - Pass 2: equal diagonals use independent per-endpoint face selection (mixed face pairs)
      Each endpoint picks face with lowest load; tiebreaker prefers vertical.
      This pushes L-route bends to layout corners, keeping center clean.
- [x] 4.3 Implement port assignment — collect edges per (node, face), sort by neighbor position, spread evenly using `L * (i+1) / (N+1)` formula
  - Minimum clearance: 8px between adjacent ports, 12px from face corners
  - Port alignment: for straight edges (same row/col), align both ports to the face with FEWER ports (it's centered)
- [x] 4.4 Implement straight-line route — same row/col, adjacent cells → 2 points
- [x] 4.5 Implement L-route — diagonal cells → 3 points, 1 bend
  - Face-aware orientation: first segment matches srcFace exit direction
    (BOTTOM/TOP → vertical-first, LEFT/RIGHT → horizontal-first)
  - Falls back to alt orientation if primary crosses a node
- [x] 4.6 Implement crossing detection — AABB check against occupied cell bounding boxes (4px margin)
- [x] 4.7 Implement Z-route — when straight/L crosses occupied cell → 4 points, 2 bends through channel
  - Channel at midpoint between rows/cols
  - Perpendicular and opposite fallbacks if primary Z crosses
- [x] 4.8 Wire into `Layout()` — replace `RouteEdges()` call with `gridRouteEdges()`
- [x] 4.9 Set `edge.LabelPosition = OutsideTopCenter` for edge labels
- [x] 4.10 Skip `TraceToShape` — ports are already on shape boundaries
- [x] 4.11 Test: adjacent cells produce straight lines
- [x] 4.12 Test: diagonal cells produce L-routes with 1 bend
- [x] 4.13 Test: edge crossing occupied cell upgrades to Z-route
- [x] 4.14 Test: multiple edges to same face are spread evenly with min clearance
- [x] 4.15 Test: no edge routes pass through node bounding boxes
- [ ] 4.16 KNOWN ISSUE: non-equal diagonals (|dc|≠|dr|) with long horizontal dominant
  distance produce L-routes with long bridge segments through empty space.
  Also: multi-cell diagonals (distance >=2 in both axes) through dense grids
  may cross intermediate nodes. Fix requires extended face selection or maze routing.

## 5. Implement Stage 4: Path ordering (for Dijkstra router) [DONE]
- [x] 5.1 Create `d2layouts/d2wueortho/ordering.go`
- [x] 5.2 Identify shared segments (multiple edges using the same routing graph edge)
- [x] 5.3 Assign default directions: LEFT for horizontal bundles, DOWN for vertical
- [x] 5.4 Order edges within each bundle by source/destination positions
- [x] 5.5 Ensure crossings only at bend points (no extra bends introduced)
- [x] 5.6 Integrated into RouteEdges pipeline (called between routing and nudging)

## 6. Upgrade Stage 5: Constraint-based nudging (for Dijkstra router) [DONE]
- [x] 6.1 Build constraint DAG: order segments with min-distance arcs
- [x] 6.2 Add minimum-distance arcs between adjacent segments (10px spacing)
- [x] 6.3 Transitive reduction: chain topology makes this implicit (no redundant arcs)
- [x] 6.4 Solve via topological sort + longest-path
- [x] 6.5 Apply computed positions to route segments, centered in channel
- [x] 6.6 Keep existing even-distribution as fallback if DAG solving fails
- [x] 6.7 Verified via existing nudging tests

## 7. Add RoutingPlugin to Dagre [DONE]
- [x] 7.1 Add `RouteEdges()` method to `dagrePlugin` in `d2plugin/plugin_dagre.go`
- [x] 7.2 Add `ROUTES_EDGES` to Dagre's feature list in `Info()`
- [x] 7.3 Add import for `d2wueortho` package
- [ ] 7.4 Test: Dagre grids now get orthogonal edge routing by default

## 8. Add grid-routing toggle [DONE]
- [x] 8.1 Add `GridRouting *bool` field to `d2target.Config` (`d2target/d2target.go`)
- [x] 8.2 Add `grid-routing` keyword parsing in `d2compiler/compile.go` `compileConfig()`
- [x] 8.3 No changes needed to `d2ast/keywords.go` (d2-config keys are not validated as reserved keywords)
- [x] 8.4 In `d2lib/d2.go`: apply config — if `GridRouting == false`, override edgeRouter to `DefaultRouter`
- [x] 8.5 Add `--grid-routing` CLI flag in `d2cli/main.go`
- [ ] 8.6 Test: `grid-routing: false` disables orthogonal routing
- [ ] 8.7 Test: default behavior enables orthogonal routing

## 9. Integration with `d2layouts.LayoutNested()`
- [ ] 9.1 Verify wueortho's `Layout()` works with LayoutNested dispatch
- [ ] 9.2 Ensure `edgeRouter` from wueortho plugin is passed correctly
- [ ] 9.3 Test nested diagrams: layers, scenarios, steps with wueortho

## 10. Testing & Visual Verification
- [ ] 10.1 Build dashboard test suite — diverse diagrams for comparison:
  - Star graphs (one central node, many satellites)
  - Linear chains (A→B→C→D)
  - Cycles (A→B→C→A)
  - Dense meshes (many interconnections)
  - Varying node sizes (small + large nodes)
  - Containers/groups
- [ ] 10.2 Compare wueortho vs dagre vs elk on all test diagrams
- [ ] 10.3 Visual verification checklist:
  - Nodes grid-aligned (same row = same Y, same col = same X)
  - Max 2 bends per edge
  - Bends near target shapes, not in empty space
  - Labels readable, offset from edges
  - No edges through node boxes
  - Symmetric placement for symmetric graphs
- [ ] 10.4 E2E: `--layout wueortho` produces valid SVG
- [ ] 10.5 E2E: grid diagram with dagre gets orthogonal edges
- [ ] 10.6 E2E: `grid-routing: false` falls back to straight lines
- [ ] 10.7 E2E: containers/nesting with wueortho
- [ ] 10.8 No NaN values in SVG paths
- [ ] 10.9 Build passes: `go build ./...`
- [ ] 10.10 All existing tests pass: `go test ./...`
- [ ] 10.11 Accept new golden files: `TESTDATA_ACCEPT=1`

## BACKLOG (future, not in this change)
- [ ] B.1 Rip-up and reroute — route all edges, find worst crossings, reroute offenders
  - RESEARCH DONE: standard VLSI technique, 10-30% crossing reduction
  - Route all → count crossings per edge → remove worst → re-route with updated penalties → repeat 2-3x
- [ ] B.2 Iterative port reordering — after routing, recompute port order using actual route positions
  - RESEARCH DONE: ELK does this with LAYER_SWEEP, converges in 2-3 iterations
  - Recompute barycenter from routed positions, not just neighbor centers
- [ ] B.3 Zero-crossing edge routing — PCB-like, edges never cross each other
  - RESEARCH DONE: requires sequential routing with obstacle maps + rip-up/reroute
  - Maze routing (Lee algorithm), channel routing from VLSI
  - Build on B.1 (rip-up and reroute) as prerequisite
- [ ] B.4 Container-aware grid placement
  - Layout inside groups first (each group is a sub-grid)
  - Treat groups as multi-cell units in outer grid
  - Prototype needed — changes cell sizing significantly
- [ ] B.5 Adaptive channel width — increase when many parallel edges share a corridor
- [ ] B.6 Edge label collision avoidance — detect label-node and label-label overlaps
- [ ] B.7 Topology detection — detect chains, stars, trees and use specialized sub-layouts
  - RESEARCH DONE: yFiles orthogonal layout and TopoLayout (Archambault 2007) do this
  - Chain detection (degree-2 paths → straight line), star detection (high-degree center), tree detection
  - ~100 lines, medium effort, medium impact
- [ ] B.8 Stress majorization + grid snap — alternative placement to BFS for complex graphs
  - RESEARCH DONE: Kieffer et al. (2013) grid-snap penalty in stress function
  - Biedl & Derka (2016) optimal snapping via min-cost matching
  - Better than BFS for graphs where topology doesn't map cleanly to BFS distance
  - ~300 lines, significant effort
- [ ] B.9 Quality metrics dashboard — automated regression testing for placement quality
  - Metrics: total edge length, crossing count, compactness, edge length variance, aspect ratio
  - Run on each code change to prevent quality regressions
