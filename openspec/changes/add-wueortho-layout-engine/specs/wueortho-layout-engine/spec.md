## ADDED Requirements

### Requirement: Wueortho Layout Engine Registration
The system SHALL register wueortho as a bundled layout plugin, selectable alongside dagre and ELK. The plugin name SHALL be "wueortho".

#### Scenario: CLI layout selection
- **WHEN** user passes `--layout wueortho`
- **THEN** the wueortho layout engine SHALL be used for node positioning and edge routing

#### Scenario: D2 syntax layout selection
- **WHEN** user writes `vars: { d2-config: { layout-engine: wueortho } }`
- **THEN** the wueortho layout engine SHALL be used for node positioning and edge routing

#### Scenario: Layout listing
- **WHEN** user runs `d2 layout`
- **THEN** wueortho SHALL appear in the list of available layout engines

#### Scenario: Build tag exclusion
- **WHEN** the binary is compiled with `nowueortho` build tag
- **THEN** the wueortho plugin SHALL NOT be registered

### Requirement: Wueortho Grid-Snap BFS Node Positioning
When used as a standalone layout engine, wueortho SHALL position nodes on a virtual grid using BFS traversal from the most-connected node, producing aligned, compact, symmetric layouts.

#### Scenario: Default diagram layout
- **WHEN** a non-grid, non-sequence diagram is laid out with wueortho
- **THEN** nodes SHALL be placed on a virtual grid where each cell fits the largest node plus a routing channel
- **AND** BFS SHALL start from the node with the highest degree (most connections)
- **AND** connected nodes SHALL be placed in adjacent grid cells
- **AND** higher-degree neighbors SHALL be placed first (better positions)
- **AND** all nodes SHALL be axis-aligned (same row = same Y, same column = same X)

#### Scenario: Grid cell sizing
- **WHEN** nodes have varying sizes
- **THEN** each grid cell SHALL be sized as max(nodeWidth, nodeHeight) + channel
- **AND** channel width SHALL be sufficient for edge routing between cells (default 80px)
- **NOTE** RESEARCH: variable cell sizes (per-row/per-column) may replace uniform sizing

#### Scenario: Disconnected components
- **WHEN** a diagram has disconnected components (nodes with no shared edges)
- **THEN** disconnected nodes SHALL be placed in remaining free cells adjacent to existing placement

#### Scenario: Symmetric placement
- **WHEN** a graph has symmetric topology (e.g., star graph with one center and N satellites)
- **THEN** placement SHALL reflect the symmetry (e.g., cross/plus shape for a star)

#### Scenario: Deterministic output
- **WHEN** the same diagram is laid out multiple times
- **THEN** the result SHALL be identical (deterministic: same input → same output)

#### Scenario: Grid diagram delegation
- **WHEN** a grid diagram is laid out with wueortho
- **THEN** node positioning SHALL be handled by the existing grid layout (d2grid)
- **AND** edge routing SHALL use wueortho's orthogonal router (Hegemann & Wolff stages 2–5)

#### Scenario: Sequence diagram delegation
- **WHEN** a sequence diagram is laid out with wueortho
- **THEN** layout SHALL be delegated to the existing sequence diagram handler (d2sequence)

### Requirement: Wueortho L/Z Edge Router (Standalone Mode)
When used as a standalone layout engine, wueortho SHALL route edges using a grid-aware L/Z-router that produces clean orthogonal routes with bends near shape entries.

#### Scenario: Face selection by direction
- **WHEN** an edge connects two nodes on the same row
- **THEN** the source SHALL exit from RIGHT or LEFT face, and destination SHALL enter from LEFT or RIGHT face (opposite direction)
- **WHEN** an edge connects two nodes on the same column
- **THEN** the source SHALL exit from BOTTOM or TOP, and destination SHALL enter from TOP or BOTTOM
- **WHEN** an edge connects diagonal nodes
- **THEN** the dominant axis (larger displacement) SHALL determine face selection

#### Scenario: Port assignment on faces
- **WHEN** multiple edges use the same face of a node
- **THEN** ports SHALL be distributed evenly along the face: position = face_start + L * (i+1) / (N+1)
- **AND** ports SHALL be sorted by neighbor position (spatial order minimizes visual crossings)
- **AND** a single port SHALL be at the center of the face

#### Scenario: Straight-line route
- **WHEN** source and destination are on the same row or column in adjacent cells
- **THEN** the route SHALL be a straight line (2 points, 0 bends)

#### Scenario: L-route
- **WHEN** source and destination are in diagonal cells (different row AND column)
- **THEN** the route SHALL have exactly 1 bend (3 points)
- **AND** the bend SHALL be near the target shape (last segment is the "approach" into the entry face)

#### Scenario: Z-route (obstacle avoidance)
- **WHEN** a straight or L-route would cross through an occupied cell's bounding box
- **THEN** the route SHALL be upgraded to a Z-route with 2 bends (4 points)
- **AND** the route SHALL pass through the channel between cell rows/columns

#### Scenario: No edge-through-node crossings
- **WHEN** edges are routed
- **THEN** no edge segment SHALL pass through any node's bounding box

#### Scenario: Maximum bend count
- **WHEN** any edge is routed in standalone mode
- **THEN** the route SHALL have at most 2 bends (4 points maximum)

#### Scenario: Edge labels
- **WHEN** an edge has a label
- **THEN** the label SHALL be positioned at OutsideTopCenter (offset above the edge line)

### Requirement: Wueortho CLI Flags
The wueortho plugin SHALL expose configurable parameters as CLI flags.

#### Scenario: Crossing penalty
- **WHEN** user passes `--wueortho-crossingPenalty <value>`
- **THEN** the specified penalty SHALL be applied to edge crossings during Dijkstra routing (grid mode)
- **AND** default value SHALL be 500

#### Scenario: Edge spacing
- **WHEN** user passes `--wueortho-edgeSpacing <value>`
- **THEN** the minimum spacing between parallel edges SHALL be the specified value
- **AND** default value SHALL be 10

**NOTE**: Some CLI flags from the FR-based design (iterations, seed, overlapRemoval) are no longer relevant with grid-snap placement. They may be replaced with grid-specific parameters as the design stabilizes.

### Requirement: Default Grid Edge Routing
Grid diagrams SHALL receive orthogonal edge routing by default, regardless of which layout engine is selected.

#### Scenario: Dagre with grid routing
- **WHEN** the layout engine is dagre
- **AND** the diagram contains a grid
- **THEN** edges within the grid SHALL be routed orthogonally via wueortho (Hegemann & Wolff stages 2–5)
- **AND** routes SHALL avoid crossing through node boxes

#### Scenario: ELK with grid routing
- **WHEN** the layout engine is ELK
- **AND** the diagram contains a grid
- **THEN** edges within the grid SHALL be routed orthogonally via wueortho (unchanged behavior)

#### Scenario: Wueortho with grid routing
- **WHEN** the layout engine is wueortho
- **AND** the diagram contains a grid
- **THEN** edges within the grid SHALL be routed orthogonally via wueortho

### Requirement: Grid Routing Toggle
Users SHALL be able to disable orthogonal grid edge routing via configuration.

#### Scenario: Disable via D2 config
- **WHEN** user writes `vars: { d2-config: { grid-routing: false } }`
- **THEN** grid edge routing SHALL fall back to straight lines (center-to-center)
- **AND** this SHALL apply regardless of the selected layout engine

#### Scenario: Disable via CLI flag
- **WHEN** user passes `--grid-routing=false`
- **THEN** grid edge routing SHALL fall back to straight lines

#### Scenario: Default enabled
- **WHEN** no grid-routing configuration is specified
- **THEN** orthogonal grid edge routing SHALL be enabled by default

### Requirement: Design Philosophy
The wueortho standalone layout SHALL produce diagrams that are compact, symmetric, and immediately readable.

#### Scenario: Invisible grid alignment
- **WHEN** nodes are positioned
- **THEN** the human eye SHALL perceive an orderly system behind the placement (axis-aligned grid)

#### Scenario: Bends near entry, not in empty space
- **WHEN** an edge route has a bend
- **THEN** the bend SHALL be near the target shape (the "approach" into the port)
- **AND** bends SHALL NOT occur in the middle of empty space

#### Scenario: Edge entry by direction
- **WHEN** an arrow comes from a left neighbor
- **THEN** it SHALL enter from the left face of the target
- **AND** center of face SHALL be preferred, nearby position if center is occupied

### RESEARCH Items
The following areas require investigation before or during implementation:

1. **Variable grid cell sizes** — per-row/per-column sizing instead of uniform max. Reduces wasted space when nodes vary in size.
2. **BFS direction priority** — should {right, down, left, up} adapt to graph topology? Linear chains → single row, stars → radial expansion.
3. **Aspect ratio control** — prevent layouts from being too wide or too tall. Possible: choose columns based on sqrt(N).
4. **L-route bend placement** — always near target? Sometimes near source? Needs visual testing across diverse diagrams.
5. **Z-route channel selection** — when detouring around occupied cells, how to choose between available channels. Heuristics for congested channels.
6. **Port order optimization** — when does simple spatial sorting fail? May need Barycenter/Median heuristics.
7. **Edge routing order** — route shorter edges first? Most constrained first?
8. **Zero-crossing routing (backlog)** — PCB-like routing where edges never cross each other.
9. **Container-aware placement (backlog)** — groups affect grid: layout inside groups first, treat groups as multi-cell units.
10. **Edge label collision avoidance** — detect label-node and label-label overlaps.
