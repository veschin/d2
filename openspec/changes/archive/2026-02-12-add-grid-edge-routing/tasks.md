## 1. Extend ELK Configuration [DONE]
- [x] 1.1 Expand `ConfigurableOpts` with all hardcoded `elkOpts` values
- [x] 1.2 Update `DefaultOpts` with current hardcoded values as defaults
- [x] 1.3 Update `Layout()` to use `opts.*` instead of hardcoded values
- [x] 1.4 Add 9 new CLI flags in `d2plugin/plugin_elk.go`
- [x] 1.5 Verify `HydrateOpts` JSON deserialization works
- [x] 1.6 Test: existing behavior unchanged (default values match previous hardcoded values)

## 2. ELK Edge Router [FAILED — REMOVED]
~~Created `d2elkedgerouter` package. ELK's layered algorithm cannot preserve fixed node positions.~~
~~All edge routes crossed through nodes regardless of INTERACTIVE strategies or coordinate translation.~~
- [x] ~~2.1-2.5 Implementation (completed but broken)~~
- [x] 2.7 Remove `d2layouts/d2elkedgerouter/` package
- [x] 2.8 Remove `d2layouts/d2elklayout/js.go` (was needed only for d2elkedgerouter)

## 3. RoutingPlugin Integration [DONE]
- [x] 3.1 Add `RouteEdges` method to `elkPlugin`
- [x] 3.2 Add `ROUTES_EDGES` to ELK plugin features
- [x] 3.3 Verify `RouterResolver` picks up the router
- [x] 3.4 Update `elkPlugin.RouteEdges` to call `d2gridrouter.RouteEdges` instead of `d2elkedgerouter`

## 4. Grid Layout Wiring [DONE]
- [x] 4.1 Add `edgeRouter` parameter to `d2grid.Layout()` signature
- [x] 4.2 Replace straight-line routing with `edgeRouter` call + fallback
- [x] 4.3 Update `d2layouts.go` to pass `edgeRouter` to `d2grid.Layout()`
- [x] 4.4 Ensure edge routing happens before `gd.shift()`

## 5. Implement Grid Router — Types & Data Structures [DONE]
Based on Hegemann & Wolff (2023), arXiv:2309.01671.
Reference implementation studied: github.com/WueGD/wueortho (Scala 3).
- [x] 5.1 Create `d2layouts/d2gridrouter/types.go` — Port, Channel, RoutingGraph, EdgePath, DijkstraState types
- [x] 5.2 Create `d2layouts/d2gridrouter/router.go` — `RouteEdges()` entry point orchestrating the pipeline
- [x] 5.3 Define types: Direction, Orientation, Rect, Segment, RoutingGraphNode/Edge

## 6. Implement Grid Router — Stage 2: Port Assignment [DONE — MVP]
Paper §3.2. Determine where edges exit/enter cell boundaries.
- [x] 6.1 Create `d2layouts/d2gridrouter/portassign.go`
- [x] 6.2 Implement side determination: angle-based dominant direction
- [x] 6.3 Implement Z-shape avoidance: threshold-based L-shape preference
- [x] 6.4 Implement port ordering: sort by neighbor center coordinate
- [x] 6.5 Implement even port distribution along each side
- [x] 6.6 Handle self-loops: assign neighboring ports on least populated side

## 7. Implement Grid Router — Stage 3a: Channel Construction [DONE — MVP]
Paper §3.3. Find routing corridors between node boxes.
- [x] 7.1 Create `d2layouts/d2gridrouter/channels.go`
- [x] 7.2 Implement vertical channel finding via free-strip detection between box boundaries
- [x] 7.3 Implement horizontal channel finding (symmetric)
- [x] 7.4 Add boundary channels (bbox with margin as virtual boundary)
- [x] 7.5 Implement channel representative selection (center line, with port-aligned preference)
- [x] 7.6 Implement channel pruning: remove dominated channels (projection containment)
- **Note**: Simplified vs paper — uses boundary-based free-strip detection instead of full sweep-line. Sufficient for regular grids.

## 8. Implement Grid Router — Stage 3b: Routing Graph [DONE — MVP]
Paper §3.3. Build partial grid from channels.
- [x] 8.1 Create `d2layouts/d2gridrouter/routinggraph.go`
- [x] 8.2 Compute intersection points of all horizontal and vertical representatives
- [x] 8.3 Build graph vertices (ports + intersection points + segment endpoints)
- [x] 8.4 Build graph edges (consecutive vertices along representatives, with box-crossing check)
- [x] 8.5 Verify: edgePassesThroughBox prevents routes through node boxes

## 9. Implement Grid Router — Stage 3c: Edge Routing [DONE]
Paper §3.3. Route each edge via modified Dijkstra.
- [x] 9.1 Create `d2layouts/d2gridrouter/dijkstra.go`
- [x] 9.2 Implement modified Dijkstra: state = (length, bends, direction), lexicographic minimization
- [x] 9.3 Route all edges through routing graph with FindNearest port-to-node mapping
- [x] 9.4 Implement crossing reduction: sequential routing with crossing penalties (+500 weight on edges that would cross already-routed edges)
- [x] 9.5 Convert routing graph paths to geo.Point routes with simplifyRoute (collinear removal)
- [x] 9.6 Port positions used directly (no TraceToShape — prevents boundary kinks)
- **Note**: Uses Dijkstra instead of A* (paper uses A* with Manhattan heuristic). Functionally equivalent; A* would be faster for large graphs.

## 10. Implement Grid Router — Stage 4: Path Ordering [SKIPPED]
Paper §3.4. Order edges on shared segments (modified Pupyrev et al.).
- **Note**: Skipped for now. Crossing reduction in Stage 3c (sequential routing with penalties) provides acceptable results. Full path ordering would further improve quality for dense edge bundles.

## 11. Implement Grid Router — Stage 5: Constrained Nudging [DONE — SIMPLIFIED]
Paper §3.5. Balance inter-edge distances via constraint solving.
- [x] 11.1 Create `d2layouts/d2gridrouter/nudging.go`
- [x] 11.2 Decompose routes into orthogonal segments, group by shared corridor (same orientation + fixed coordinate)
- [x] 11.3 Even distribution: N edges in corridor of width W → position_i = chMin + W*(i+1)/(N+1)
- [x] 11.4 Find channel bounds from channel list for each bundle
- [x] 11.5 Apply nudge offsets to route points in-place
- **Note**: Simplified vs paper — uses even distribution across channel width instead of LP-based constraint solving. Sufficient for regular grids; LP would improve results for irregular layouts.

## 12. Integration & Cleanup [DONE]
- [x] 12.1 Update `d2plugin/plugin_elk.go` RouteEdges to call `d2gridrouter.RouteEdges`
- [x] 12.2 Remove `d2elkedgerouter` package and `d2elklayout/js.go`
- [x] 12.3 Handle edge labels: position along routed path (InsideMiddleCenter)
- [x] 12.4 Verify fallback: when router fails, straight-line routing still works
- [x] 12.5 Auto-increase grid gap when edges exist: `adjustGridGapForEdges()` in layout.go
- [x] 12.6 Gap formula: max(DEFAULT_GAP, EDGE_ROUTING_MIN_GAP + sqrt(edgeCount) * EDGE_ROUTING_GAP_PER_EDGE)

## 13. Testing & Verification [DONE]
- [x] 13.1 Visual verification: orthogonal routes through corridors, no node crossings
- [x] 13.2 Visual verification: edge separation (nudging) — edges evenly spaced in shared corridors
- [x] 13.3 Visual verification: crossing reduction — sequential routing with penalties reduces crossings
- [x] 13.4 Visual verification: port alignment — no kinks at shape boundaries
- [x] 13.5 No NaN values in generated SVG paths
- [x] 13.6 Build passes: `go build ./...`
- [x] 13.7 Grid tests pass: `go test ./d2layouts/d2grid/...`
- [x] 13.8 Core tests pass: compiler, parser, exporter, SVG renderer, sequence layout
- [x] 13.9 E2E tests pass: `go test ./e2etests/...`
- [x] 13.10 Unit tests for individual stages (port assignment, channels, routing graph, Dijkstra)
