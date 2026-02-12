# grid-layout Specification

## Purpose
TBD - created by archiving change add-grid-edge-routing. Update Purpose after archive.
## Requirements
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

