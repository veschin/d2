## MODIFIED Requirements

### Requirement: ELK RoutingPlugin Implementation
The ELK layout plugin SHALL implement the `RoutingPlugin` interface to provide edge routing for pre-positioned graphs via wueortho (renamed from d2gridrouter).

#### Scenario: ELK advertises routing capability
- **WHEN** the ELK plugin reports its features
- **THEN** the feature list SHALL include `ROUTES_EDGES`

#### Scenario: RouterResolver discovers ELK router
- **WHEN** the layout engine is ELK
- **AND** the system needs an edge router
- **THEN** the ELK routing plugin SHALL be selected automatically via `RouterResolver`
- **AND** it SHALL delegate to `d2wueortho.RouteEdges` (Hegemann & Wolff stages 2–5)

## ADDED Requirements

### Requirement: Dagre RoutingPlugin Implementation
The Dagre layout plugin SHALL implement the `RoutingPlugin` interface to provide orthogonal edge routing for grid diagrams via wueortho.

#### Scenario: Dagre advertises routing capability
- **WHEN** the Dagre plugin reports its features
- **THEN** the feature list SHALL include `ROUTES_EDGES`

#### Scenario: RouterResolver discovers Dagre router
- **WHEN** the layout engine is Dagre
- **AND** the system needs an edge router
- **THEN** the Dagre routing plugin SHALL be selected automatically via `RouterResolver`
- **AND** it SHALL delegate to `d2wueortho.RouteEdges` (Hegemann & Wolff stages 2–5)

#### Scenario: Dagre grid edges are routed orthogonally
- **WHEN** a grid diagram is laid out with Dagre
- **AND** grid routing is not disabled
- **THEN** edges SHALL be routed through orthogonal corridors avoiding node boxes
- **AND** the result SHALL match the quality of ELK grid edge routing

### Requirement: Two Routing Systems
The wueortho package SHALL maintain two distinct edge routing systems for different use cases.

#### Scenario: Grid routing (ELK/Dagre use case)
- **WHEN** wueortho routes edges for a grid diagram positioned by d2grid (via ELK or Dagre)
- **THEN** it SHALL use the Hegemann & Wolff pipeline (stages 2–5: port assignment, channel construction, Dijkstra routing graph, nudging)
- **AND** this is the existing `RouteEdges()` entry point in `router.go`

#### Scenario: Standalone routing (wueortho layout engine)
- **WHEN** wueortho is used as the layout engine (`--layout wueortho`)
- **AND** nodes are positioned by grid-snap BFS
- **THEN** it SHALL use the L/Z-router (`gridRouteEdges()`) which produces clean orthogonal routes with bends near shape entries
- **AND** the Hegemann & Wolff Dijkstra router SHALL NOT be used for standalone mode (it produces bends at arbitrary routing graph vertices)

#### Scenario: Routing system selection rationale
- The L/Z-router is simpler and produces cleaner results for grid-aligned nodes
- The Dijkstra router (H&W stages 2–5) is designed for arbitrary node positions from d2grid
- Both systems coexist in the same package, serving different entry points
