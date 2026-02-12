# Change: Add wueortho as a full layout engine and default grid edge router

## Why

The orthogonal edge router (`d2gridrouter`) based on Hegemann & Wolff (2023) works well for grid diagrams but has two limitations:

1. **Accessibility**: It's only available through the ELK plugin. Users on Dagre (the default engine) get ugly straight-line edges in grids. There's no way to select the router independently.

2. **Scope**: The current implementation only covers stages 2–5 of the wueortho pipeline (port assignment → edge routing → nudging). It can't function as a standalone layout engine that manages the entire diagram.

The reference implementation [wueortho](https://github.com/WueGD/wueortho) (Scala 3) and the paper (arXiv:2309.01671) describe a complete pipeline that handles both node placement and edge routing. We implement the missing stages and promote wueortho to a first-class layout engine.

## Design Philosophy

The wueortho standalone layout should produce diagrams that are **compact, symmetric, and immediately readable**. Key principles (from user interview):

- **Invisible grid** — nodes are placed on a virtual grid. The human eye reads the _system_ behind the placement, and that systemicity is what makes diagrams readable. Not a literal d2 grid — an aligned placement strategy.
- **Symmetry** — 3 nodes connected to one central node should be arranged symmetrically around it. The layout reflects the topology.
- **PCB-like routing (but softer)** — edges as clean orthogonal traces: strictly horizontal/vertical, minimal bends, avoid crossings where reasonable. But don't route a line across 1000px to avoid one crossing — balance cleanliness with compactness.
- **Bends near entry, not in empty space** — the 90° turn happens right before entering the shape. The last segment is the "approach into the port". No bends in the middle of nowhere.
- **Edge entry by direction** — arrow from a left neighbor enters from the left face. Center of face first, nearby position if center is occupied.
- **No hierarchy required** — wueortho doesn't need top-down flow. Star/cross/organic layouts are fine — what matters is that edges are readable and nodes are aligned.

## What Changes

### 1. Rename `d2gridrouter` → `d2wueortho` [DONE]
The package is no longer just a grid router — it implements the full Hegemann & Wolff pipeline. The name `wueortho` matches the reference implementation.

### 2. wueortho as a standalone layout engine [PARTIALLY DONE]
- Register `wueorthoPlugin` in `d2plugin/plugin_wueortho.go` (like dagre/elk) [DONE]
- Selectable via `--layout wueortho` and `vars.d2-config.layout-engine: wueortho` [DONE]
- Build tag `!nowueortho` for conditional compilation [DONE]
- Implements node placement + edge routing:
  - **Node placement**: Grid-snap BFS — place nodes on a virtual grid using BFS from the most-connected node. Connected nodes get adjacent cells. [IN PROGRESS]
  - **Edge routing**: Simple L/Z-router — orthogonal routes with bends near entry points. Straight lines for same-row/column, L-routes (1 bend) for diagonal, Z-routes (2 bends) when avoiding occupied cells. [TODO]
  - Existing Dijkstra pipeline (stages 2–5) stays untouched for grid routing use case
- Own CLI flags for configuration [DONE]

### 3. wueortho as the default grid edge router
- Grid diagrams automatically get orthogonal edge routing via wueortho regardless of the selected layout engine (dagre, elk, or wueortho)
- This is enabled by default with an option to disable (`--grid-routing=false` or `vars.d2-config.grid-routing: false`)
- When disabled, falls back to straight-line routing (current dagre behavior)
- Dagre and ELK both implement `RoutingPlugin` delegating to `d2wueortho.RouteEdges()`

### 4. Improve existing pipeline stages
- Stage 4 (path ordering): Implement properly instead of relying solely on crossing penalties
- Stage 5 (nudging): Upgrade from even distribution to LP-based constraint solving (DAG longest-path)

## Evolution Log

### v1: FR-based layout (initial)
Used Fruchterman-Reingold for node positioning + GTree overlap removal + Dijkstra routing graph. Produced unreadable diagrams: nodes at arbitrary positions, bends in empty space, no alignment.

### v2: Grid-snap + L/Z-router (current direction)
Replace FR with BFS grid placement. Replace Dijkstra router (for standalone mode) with simple L/Z-router that produces clean routes with bends near shape entries. Dijkstra router stays for grid routing (ELK/dagre). Design philosophy: invisible grid alignment, PCB-like edge routing, symmetry.

## Backlog (future improvements)

1. **Zero-crossing edge routing** — edges route around each other like PCB traces. No edge-to-edge crossings at all. Requires crossing detection + rerouting.
2. **Container-aware placement** — groups affect the grid: layout inside groups first, then treat groups as multi-cell units in the outer grid.
3. **Adaptive channel width** — increase channel between cells when many parallel edges share a corridor.
4. **Edge label collision avoidance** — check if labels overlap nodes or other labels, adjust placement.
5. **Aspect ratio optimization** — choose grid dimensions that produce a balanced (not too wide/tall) diagram.

## Impact
- Affected specs: `grid-layout` (modified), `wueortho-layout-engine` (new capability)
- Affected code:
  - `d2layouts/d2gridrouter/` → renamed to `d2layouts/d2wueortho/` [DONE]
  - `d2layouts/d2wueortho/layout.go` (NEW — grid-snap placement + routing dispatch)
  - `d2layouts/d2wueortho/gridroute.go` (NEW — L/Z-router for standalone mode)
  - `d2plugin/plugin_wueortho.go` (NEW — plugin registration) [DONE]
  - `d2plugin/plugin_dagre.go` (MODIFIED — add RoutingPlugin support)
  - `d2plugin/plugin_elk.go` (MODIFIED — update import path) [DONE]
  - `d2compiler/compile.go` (MODIFIED — parse `grid-routing` config)
  - `d2target/d2target.go` (MODIFIED — add GridRouting to Config)
  - `d2lib/d2.go` (MODIFIED — default grid routing logic)
  - `d2cli/main.go` (MODIFIED — new flags)
- Backward compatible: existing diagrams render the same; dagre grids get better edges by default
- Fork-specific: all changes marked with `// [FORK]` comments
