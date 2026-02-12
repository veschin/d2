# elk-configuration Specification

## Purpose
TBD - created by archiving change add-grid-edge-routing. Update Purpose after archive.
## Requirements
### Requirement: Extended ELK Layout Configuration
The ELK layout engine SHALL expose all rendering-relevant parameters as CLI flags, replacing previously hardcoded values with configurable options that default to the current hardcoded values.

#### Scenario: Thoroughness configuration
- **WHEN** user passes `--elk-thoroughness <value>`
- **THEN** ELK SHALL use the specified thoroughness value for layout computation
- **AND** default value SHALL be 8 (matching previous hardcoded value)

#### Scenario: Edge-edge spacing configuration
- **WHEN** user passes `--elk-edgeEdgeBetweenLayers <value>`
- **THEN** ELK SHALL use the specified spacing between edges routed between layers
- **AND** default value SHALL be 50

#### Scenario: Edge-node spacing configuration
- **WHEN** user passes `--elk-edgeNode <value>`
- **THEN** ELK SHALL use the specified spacing between edges and nodes
- **AND** default value SHALL be 40

#### Scenario: Fixed alignment configuration
- **WHEN** user passes `--elk-fixedAlignment <value>`
- **THEN** ELK SHALL use the specified alignment strategy (NONE, LEFTUP, RIGHTUP, LEFTDOWN, RIGHTDOWN, BALANCED)
- **AND** default value SHALL be "BALANCED"

#### Scenario: Model order strategy configuration
- **WHEN** user passes `--elk-considerModelOrder <value>`
- **THEN** ELK SHALL use the specified model order strategy (NONE, NODES_AND_EDGES, PREFER_EDGES, PREFER_NODES)
- **AND** default value SHALL be "NODES_AND_EDGES"

#### Scenario: Cycle breaking strategy configuration
- **WHEN** user passes `--elk-cycleBreakingStrategy <value>`
- **THEN** ELK SHALL use the specified cycle breaking strategy (GREEDY, GREEDY_MODEL_ORDER, DEPTH_FIRST, INTERACTIVE)
- **AND** default value SHALL be "GREEDY_MODEL_ORDER"

#### Scenario: Crossing minimization configuration
- **WHEN** user passes `--elk-crossingMinimization <value>`
- **THEN** ELK SHALL use the specified crossing minimization strategy (LAYER_SWEEP, INTERACTIVE)
- **AND** default value SHALL be "LAYER_SWEEP"

#### Scenario: Node placement strategy configuration
- **WHEN** user passes `--elk-nodePlacement <value>`
- **THEN** ELK SHALL use the specified node placement strategy (BRANDES_KOEPF, LINEAR_SEGMENTS, SIMPLE, NETWORK_SIMPLEX)
- **AND** default value SHALL be "BRANDES_KOEPF"

#### Scenario: Edge routing style configuration
- **WHEN** user passes `--elk-edgeRouting <value>`
- **THEN** ELK SHALL use the specified edge routing style (ORTHOGONAL, POLYLINE, SPLINES)
- **AND** default value SHALL be "ORTHOGONAL"

#### Scenario: Default behavior unchanged
- **WHEN** no new ELK flags are specified
- **THEN** layout behavior SHALL be identical to the previous hardcoded behavior

