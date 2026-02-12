## ADDED Requirements

### Requirement: Grid Edge Routing via Orthogonal Router
Grid diagrams SHALL route edges between grid cells using an orthogonal routing algorithm based on Hegemann & Wolff (2023) that produces clean paths avoiding all grid cells as obstacles.

**Reference**: Tim Hegemann, Alexander Wolff. "A Simple Pipeline for Orthogonal Graph Drawing." GD 2023, LNCS 14466. arXiv:2309.01671.

#### Scenario: Grid edges routed orthogonally
- **WHEN** a grid diagram has edges between grid cells
- **AND** a routing-capable layout engine is available
- **THEN** edges SHALL be routed as orthogonal polylines (horizontal and vertical segments only)
- **AND** edge routes SHALL NOT pass through any grid cell (all cells are obstacles)
- **AND** edge routes SHALL use grid corridors (gaps between rows/columns and perimeter margins)

#### Scenario: Minimum bends
- **WHEN** edges are routed in a grid diagram
- **THEN** the router SHALL minimize the number of bends in each route
- **AND** prefer L-shaped routes (1 bend) over Z-shaped routes (2 bends) when possible

#### Scenario: Edge separation (nudging)
- **WHEN** multiple edges share a corridor segment
- **THEN** edges SHALL be offset to parallel tracks within the corridor
- **AND** inter-edge distances SHALL be balanced (not bunched together)

#### Scenario: Fallback to straight-line routing
- **WHEN** a grid diagram has edges between grid cells
- **AND** no routing-capable layout engine is available (e.g., Dagre)
- **OR** the orthogonal router fails
- **THEN** edges SHALL be routed as straight lines between cell centers (current behavior preserved)

#### Scenario: Grid edges with labels
- **WHEN** a routed grid edge has a label
- **THEN** the label SHALL be positioned along the routed path

#### Scenario: Edge routing respects grid cell positions
- **WHEN** edges are routed in a grid diagram
- **THEN** grid cell positions SHALL NOT change (only edge routes are computed)
- **AND** edge routes SHALL be correctly shifted when the grid container is repositioned

## ADDED Requirements

### Requirement: ELK RoutingPlugin Implementation
The ELK layout plugin SHALL implement the `RoutingPlugin` interface to provide edge routing for pre-positioned graphs via the grid router.

#### Scenario: ELK advertises routing capability
- **WHEN** the ELK plugin reports its features
- **THEN** the feature list SHALL include `ROUTES_EDGES`

#### Scenario: RouterResolver discovers ELK router
- **WHEN** the layout engine is ELK
- **AND** the system needs an edge router
- **THEN** the ELK routing plugin SHALL be selected automatically via `RouterResolver`
- **AND** it SHALL delegate to `d2gridrouter.RouteEdges`

## CHANGED Behavior (from original D2)

### Grid edges no longer straight lines
- **BEFORE**: All grid edges were straight center-to-center lines
- **AFTER**: Grid edges are orthogonal polylines routed through corridors between cells
- **CONDITION**: Only when using ELK layout engine; Dagre retains straight-line behavior

## DEPRECATED

### ELK-based edge routing (d2elkedgerouter)
- **STATUS**: Failed experiment, to be removed
- **REASON**: ELK's layered algorithm cannot preserve fixed node positions; edge routes cross through nodes regardless of INTERACTIVE strategy settings
- **REPLACEMENT**: `d2gridrouter` — pure Go orthogonal router based on Hegemann & Wolff (2023)
