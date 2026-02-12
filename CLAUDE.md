<!-- OPENSPEC:START -->

# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:

- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big
  performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:

- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

## OpenSpec CLI Commands

```sh
openspec list                              # List active changes
openspec show <change>                     # Show change details (proposal)
openspec change new <name>                 # Create new change
openspec validate <change>                 # Validate change artifacts
openspec archive -y <change>               # Archive completed change (auto-confirm)
openspec archive --skip-specs <change>     # Archive without updating specs
openspec spec list                         # List all specs
openspec spec show <spec>                  # Show a spec
```

Slash-command shortcuts (Claude Code skills):

| Command              | What it does                                      |
|----------------------|---------------------------------------------------|
| `/opsx:new`          | Start a new change (guided artifact creation)      |
| `/opsx:continue`     | Create next artifact for a change                  |
| `/opsx:ff`           | Fast-forward: create all artifacts at once          |
| `/opsx:apply`        | Implement tasks from a change                      |
| `/opsx:verify`       | Verify implementation matches change artifacts     |
| `/opsx:archive`      | Archive a completed change                         |
| `/opsx:bulk-archive` | Archive multiple changes at once                   |
| `/opsx:sync`         | Sync delta specs to main specs                     |
| `/opsx:explore`      | Think through ideas before/during a change         |
| `/opsx:onboard`      | Guided walkthrough of the full workflow             |

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## What is D2?

D2 is a diagram scripting language that turns text into diagrams. Written in Go,
it provides a complete text-to-diagram pipeline: parser, compiler, layout
engines, and renderers (SVG/PNG/PDF/GIF). Module path: `oss.terrastruct.com/d2`.

## Build & Development Commands

CI infrastructure lives in a git submodule. After cloning:

```sh
git submodule update --init --recursive
```

Common commands (via `./make.sh` or `make`):

```sh
./make.sh fmt       # Format Go (gofmt/goimports), JS (prettier), D2 files
./make.sh lint      # go vet --composites=false ./...
./make.sh build     # go build ./...
./make.sh test      # CI=1 go test --timeout=30m ./...
./make.sh race      # Tests with -race flag
./make.sh           # Runs all: fmt, gen, js, lint, build, test
```

Running tests directly:

```sh
go test ./d2parser                        # Single package
go test -v -run TestName ./d2compiler     # Single test
TESTDATA_ACCEPT=1 go test ./d2parser      # Accept new/changed test outputs
D2_CHAOS_MAXI=100 D2_CHAOS_N=100 go test --timeout=30m ./d2chaos  # Chaos tests
```

E2E visual regression report:

```sh
go run ./e2etests/report/main.go -delta
open ./e2etests/out/e2e_report.html
```

## Test Conventions

Most tests compare against golden files checked into version control (testdata
dirs). New tests will fail on first run because no expected output exists yet.
Run with `TESTDATA_ACCEPT=1` to accept outputs, then commit them.

E2E tests in `e2etests/` are categorized as:

- **Stable** (`stable_test.go`): No known issues
- **Regressions** (`regression_test.go`): Fixed bugs (linked to PR)
- **Todos** (`todo_test.go`): Known issues (linked to GitHub Issue)

## Architecture

**Compilation pipeline** (text → diagram):

```
D2 text
  → d2parser → d2ast (AST)
  → d2ir (resolve globs, imports, substitutions, classes)
  → d2compiler → d2graph (Object/Edge graph with attributes)
  → d2layouts (position objects, route edges)
  → d2exporter → d2target (serializable Diagram)
  → d2renderers (SVG/PNG/PDF/GIF/ASCII/PPTX)
```

Entry point: `main.go` → `d2cli.Run()`. Library entry: `d2lib.Compile()`.

---

## Module Map — Where to Change What

### d2parser (`d2parser/parse.go`)

Rune-by-rune lexer/parser. Produces AST even when errors exist (for tooling).

- **Add new syntax/delimiters** — `parseMapNode()` switch cases,
  `parseUnquotedString()`
- **Change escape sequences** — `decodeEscape()`
- **Modify comment syntax** — `parseComment()`, `parseBlockComment()`
- **Block string tags** (md, latex, etc.) — `parseBlockString()` tag parsing
  loop
- **String interpolation** (`${...}`) — `InterpolationBox` handling in
  `parseUnquotedString()`, `parseDoubleQuotedString()`
- **Edge/arrow parsing** (`->`, `<-`, `--`) — `parseEdges()`, `parseEdge()`
- **Import syntax** (`@path`) — `parseImport()`
- **Glob patterns** (`*`, `**`) — handled at parse level, expanded in d2ir
- Key exports: `Parse()`, `ParseKey()`, `ParseMapKey()`, `ParseValue()`

### d2ast (`d2ast/`)

AST node type definitions. Pure data, no logic besides traversal.

- **d2ast.go** — All node types: `Map`, `Key`, `Edge`, `KeyPath`, `Scalar`
  variants (`Null`, `Boolean`, `Number`, `UnquotedString`, `DoubleQuotedString`,
  `SingleQuotedString`, `BlockString`), `Substitution`, `Import`, `Array`,
  `Comment`, `BlockComment`. Boxing types (`MapNodeBox`, `ValueBox`,
  `ScalarBox`, `StringBox`, `InterpolationBox`). Position/Range tracking.
  `Walk()` for traversal.
- **keywords.go** — All reserved keywords:
  - `SimpleReservedKeywords` — shape, icon, label, width, height, direction,
    tooltip, link, etc.
  - `StyleKeywords` — opacity, stroke, fill, font-size, bold, animated, shadow,
    border-radius, etc.
  - `CompositeReservedKeywords` — source-arrowhead, target-arrowhead, classes
  - `BoardKeywords` — layers, scenarios, steps
  - `NearConstantsArray` — top-left, top-center, center-right, etc.
  - `LabelPositionsArray` — all label position values
- **Add a new reserved keyword** → add to the appropriate map in `keywords.go`

### d2ir (`d2ir/`)

Intermediate representation. Resolves all indirections (globs, imports,
substitutions, classes) into a flat tree of Fields and Edges.

- **d2ir.go** — Core types: `Map` (Fields + Edges + globs), `Field` (name +
  primary + composite + references), `Edge` (ID + primary + map + references),
  `Scalar`, `Array`, `EdgeID`. Reference tracking (`FieldReference`,
  `EdgeReference`, `RefContext`).
- **compile.go** — `Compile()` entry. `compileMap()` processes AST nodes into
  IR. `compileSubstitutions()` resolves `${path}` values. `overlayClasses()`
  applies class inheritance. `removeSuspendedFields()` handles
  suspend/unsuspend.
  - **Change how fields resolve** → `EnsureField()`, `eachMatching()`
  - **Change how edges resolve** → `GetEdges()`, `EnsureEdges()`
  - **Change class behavior** → `overlayClasses()`, `GetClassMap()`
- **pattern.go** — Glob expansion: `*` (one component), `**` (recursive, skip
  reserved), `***` (recursive, include board keywords). Functions:
  `multiGlob()`, `matchPattern()`.
- **import.go** — `_import()`, `__import()`. Cyclic import detection via import
  stack.
- **merge.go** — `OverlayMap()`, `OverlayField()`, `OverlayEdge()` — how
  duplicate definitions merge.
- **query.go** — `Query()`, `QueryAll()` — test/debug lookup by key string.

### d2compiler (`d2compiler/compile.go`)

Converts IR into d2graph. Validates constraints. Orchestrates parse → IR →
graph.

- `Compile()` — main entry: calls `d2parser.Parse()` → `d2ir.Compile()` →
  `compileIR()`
- **Object/edge creation** — `compileMap()` populates objects, `compileField()`
  creates children via `obj.EnsureChild()`, `compileEdge()` creates connections
- **Reserved keyword handling** — `compileReserved()`: label (markdown parsing),
  shape, icon, position (top/left/near), grid (rows/columns/gap), direction
- **Style compilation** — `compileStyleField()`: validates colors, opacity,
  stroke, fill, font properties
- **Special shapes** — `compileClass()` (UML fields/methods with +/-/#
  visibility), `compileSQLTable()` (columns with types/constraints)
- **Board nesting** — `compileBoardsField()`: creates child graphs for
  layers/scenarios/steps
- **Validation** (where to add constraints):
  - `validateKeys()` — shape-specific rules (circle width==height, 3d only on
    certain shapes, image needs icon)
  - `validateLabels()` — non-empty text shapes, no newlines in sql_table labels
  - `validateNear()` — near positioning constraints, no circular refs
  - `validateEdges()` — no self-references in grid/sequence
  - `validatePositionsCompatibility()` — no positions in hierarchy/sequence/grid
  - `validateBoardLinks()` — link target existence
- **Config** — `compileConfig()`: parses `vars.d2-config` block (theme-id,
  layout-engine, sketch, pad, center, theme overrides)
- **Language aliases** — `ShortToFullLanguageAliases` map (md→markdown,
  tex→latex, js→javascript, etc.)

### d2graph (`d2graph/`)

Mutable graph structure. Objects get positioned, edges get routed during layout.

- **d2graph.go** — Core types:
  - `Graph` — Root, Objects, Edges, Layers/Scenarios/Steps (nested Graphs),
    Theme, Legend
  - `Object` — ID, Box (TopLeft + Width + Height), Label, Style,
    Children/ChildrenArray, Shape, Class/SQLTable data, References
  - `Edge` — Src, Dst, Index, Route ([]Point), SrcArrow/DstArrow, Label,
    Attributes
  - `Attributes` — Label, Style, Icon, Tooltip, Link, Shape, Direction,
    GridRows/Columns/Gap, NearKey, Language
  - `Style` — all visual properties (Opacity, Stroke, Fill, FillPattern,
    StrokeWidth, FontSize, Bold, Italic, Animated, Shadow, 3D, DoubleBorder,
    BorderRadius, Font, TextTransform)
  - Key methods: `EnsureChild()`, `Connect()`, `AbsID()`, `SetDimensions()`,
    `ApplyTheme()`
- **layout.go** — Edge routing helpers, shape boundary tracing for connection
  endpoints, label/icon positioning on edges
- **serde.go** — JSON serialization/deserialization, graph comparison for tests
- **seqdiagram.go** — `IsSequenceDiagram()` detection (actors, notes, groups,
  spans)
- **grid_diagram.go** — `IsGridDiagram()` detection

### d2layouts (`d2layouts/`)

Layout engine coordination. Extracts nested diagrams, runs core layout,
reintegrates.

- **d2layouts.go** — `LayoutNested()` — main entry. Top-down recursion: extracts
  nested content, detects diagram type (grid/sequence/constant-near/default),
  runs core layout, reintegrates. `SaveOrder()`/`RestoreOrder()` for AST
  ordering.
- **d2dagrelayout/** — Dagre (DAG) layout. Hierarchical/tree. 4 directions
  (down/up/left/right). Default engine.
- **d2elklayout/** — ELK layout via WASM. Hierarchical with ports for SQL
  tables. Post-processes to remove S-curves and ladders in edge routing.
- **d2grid/** — Grid layout. Rows/columns with equal dimensions. Dynamic vs
  evenly distributed. Optimal split via standard deviation.
- **d2sequence/** — Sequence diagram layout. Horizontal actors, vertical
  lifelines, messages, groups, notes.
- **d2near/** — Constant-near positioning (top-left, center, bottom-right,
  etc.).
- **d2layoutfeatures/** — Feature flags: `NEAR_OBJECT`, `CONTAINER_DIMENSIONS`,
  `TOP_LEFT`.

### d2exporter (`d2exporter/export.go`)

Converts positioned d2graph.Graph → serializable d2target.Diagram.

- `Export()` — main entry. Iterates objects → `toShape()`, edges →
  `toConnection()`. Processes legend.
- `toShape()` — maps Object to Shape with styling, label, icon, class/SQL table
  data
- `toConnection()` — maps Edge to Connection with arrows, labels, route points
- `applyTheme()` — theme-specific rules (C4, mono, dots pattern, double-border)

### d2target (`d2target/`)

Serializable JSON-ready output structure for renderers.

- `Diagram` — shapes, connections, layers/scenarios/steps (nested), config,
  legend
- `Shape` — 25 types: rectangle, square, circle, oval, diamond, hexagon, cloud,
  text, code, class, sql_table, image, person, c4-person, cylinder, queue,
  package, step, page, document, parallelogram, callout, stored_data, hierarchy,
  sequence_diagram
- `Connection` — route ([]Point), src/dst IDs, arrowheads (11 types: arrow,
  triangle, diamond, circle, cross, box, cf-one/many, etc.), labels, styling
- `Config` — sketch, theme-id, dark-theme-id, pad, center, layout-engine
- `MText` — text with language, font, bold/italic/underline, dimensions
- Key methods: `BoundingBox()`, `GetBoard()`, `HashID()`

### d2renderers

- **d2svg/** (`d2svg.go`, ~3400 lines) — Main renderer. `Render()` for single
  diagram, `RenderMultiboard()` for animations/scenarios. `drawShape()`,
  `drawConnection()`, 3D effects, `ThemeCSS()`, `EmbedFonts()`, markdown text
  rendering, tooltips, fill patterns (dots, grain, lines). `RenderOpts`: Pad,
  Sketch, ThemeID, DarkThemeID, Scale, ThemeOverrides.
- **d2ascii/** — ASCII art renderer. `ASCIIartist.Render()`. Subdirs:
  `asciishapes/` (draw each shape type), `asciiroute/` (route connections),
  `asciicanvas/` (2D char grid), `charset/` (Unicode vs ASCII chars).
- **d2sketch/** — Hand-drawn style via rough.js. `LoadJS()`,
  `DefineFillPatterns()`, `Rect()`.
- **d2latex/** — LaTeX rendering via MathJax in JS runtime (Goja/V8). `Render()`
  → SVG, `Measure()` → dimensions.
- **d2fonts/** — Font management. Families: SourceSansPro, SourceCodePro,
  HandDrawn. Sizes: XS(13)..XXXL(32). `GetEncodedSubset()` for WOFF subsetting.
  `AddFontFamily()` for custom fonts.
- **d2animate/** — `Wrap()` — wraps multiple SVG frames into animated SVG with
  CSS keyframe transitions.

### d2themes (`d2themes/`)

- `Theme` — ID, Name, ColorPalette (Neutrals N1-N7, Base B1-B6, Alt
  AA2/AA4/AA5/AB4/AB5), SpecialRules
- Light themes (ID 0-99): NeutralDefault, NeutralGrey, FlagshipTerrastruct,
  CoolClassics, MixedBerryBlue, GrapeSoda, Aubergine, ColorblindClear,
  VanillaNitroCola, ShirelyTemple, EarthTones, EvergladeGreen, ButteredToast,
  DarkMauve, OrangeCreamsicle, C4, Terminal, TerminalGrayscale
- Dark themes (ID 200-299): DarkMauve, DarkFlagshipTerrastruct
- `d2themescatalog/` — `Find(id)`, `CLIString()`

### d2plugin (`d2plugin/`)

- `Plugin` interface — `Info()`, `Flags()`, `HydrateOpts()`, `Layout()`,
  `PostProcess()`
- `RoutingPlugin` interface — `RouteEdges()`
- Bundled: Dagre (`plugin_dagre.go`), ELK (`plugin_elk.go`)
- External: discovered as `d2plugin-*` binaries in $PATH
- `ListPlugins()`, `FindPlugin()`, `ListPluginFlags()`
- `serve.go` — HTTP server for binary plugin communication

### d2cli (`d2cli/`)

- **main.go** — `Run()`: flag parsing, subcommand routing (fmt, validate,
  layout, themes, play, version), render pipeline
- **watch.go** — `watcher`: fsnotify-based file watching, WebSocket live reload
  server, `watchLoop()`, `compileLoop()`
- **fmt.go** — `fmtCmd()`: format D2 files, `--check` flag
- **validate.go** — `validateCmd()`: validate D2 syntax
- **export.go** — export format handling (svg/png/pdf/pptx/gif/txt)
- Supported formats: `.svg` (default), `.png`/`.pdf` (via Playwright), `.pptx`,
  `.gif`, `.txt` (ASCII)

### d2lib (`d2lib/d2.go`)

High-level API orchestrating the full pipeline.

- `Parse()` — parse D2 text into AST
- `Compile()` — full pipeline: parse → compile → apply theme → set dimensions →
  layout → route edges → export
- `CompileOptions` — UTF16Pos, FS, MeasuredTexts, Ruler, RouterResolver,
  LayoutResolver, Layout, FontFamily, MonoFontFamily, InputPath

### d2oracle (`d2oracle/`)

Code intelligence for IDE/editor integration.

- **get.go** — read operations: `GetBoardGraph()`, `GetObj()`, `GetEdge()`,
  `GetChildrenIDs()`, `GetParentID()`, `GetObjOrder()`, `IsImportedObj()`,
  `IsImportedEdge()`
- **edit.go** — write operations: `Create()` (new object/edge), `Set()` (modify
  properties), `ReconnectEdge()`, `Delete()`. Recompiles graph after mutation.
  `OutsideScopeError` prevents modifying imports.

### d2lsp (`d2lsp/`)

- **d2lsp.go** — `GetRefRanges()` (all reference locations for a key),
  `GetBoardAtPosition()` (which board cursor is in)
- **completion.go** — `GetCompletionItems()`: autocomplete for styles, shapes,
  keywords, booleans, arrowheads, directions

### d2format (`d2format/`)

- **format.go** — `Format()`: AST → formatted D2 text. `printer` struct with
  indentation. Handles all node types, preserves comments.
- **escape.go** — string escaping utilities

### lib/ — Utilities

| Package           | Purpose                                                                                                                                                                                    |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `lib/geo`         | Geometric primitives: Point, Box, Vector, Segment, Ellipse, BezierCurve, Route. Intersection calculations.                                                                                 |
| `lib/shape`       | All 24 shape definitions. `Shape` interface: `GetType()`, `Perimeter()`, `GetInsidePlacement()`, `GetDimensionsToFit()`, `GetSVGPathData()`. One file per shape (`shape_circle.go`, etc.). |
| `lib/textmeasure` | Text measurement via TrueType fonts. `Ruler` type. Markdown measurement. Tab size 4, line height 1.15 (text) / 1.3 (code).                                                                 |
| `lib/color`       | Color parsing, `Luminance()`, `Darken()`, `IsThemeColor()`, gradient support.                                                                                                              |
| `lib/label`       | Label position enum (40+ positions: outside/inside/border x 9 grid spots).                                                                                                                 |
| `lib/svg`         | SVG path builder. `SvgPathContext`: `StartAt()`, `L()`, `C()`, `Z()`.                                                                                                                      |
| `lib/font`        | TTF/OTF → WOFF conversion, font subsetting.                                                                                                                                                |
| `lib/pdf`         | PDF export via gofpdf.                                                                                                                                                                     |
| `lib/png`         | PNG export via Playwright (headless Chromium). 2x resolution.                                                                                                                              |
| `lib/pptx`        | PowerPoint export. OOXML zip packaging. Slides with embedded images.                                                                                                                       |
| `lib/xgif`        | Animated GIF from SVG frames. 30fps, 255 colors, parallel processing.                                                                                                                      |
| `lib/imgbundler`  | Embeds external images into SVG as base64 data URIs. Caching (33MB max).                                                                                                                   |
| `lib/compression` | Brotli/gzip/zlib decompression for embedded SVG images.                                                                                                                                    |
| `lib/memfs`       | In-memory `fs.FS` for WASM environments.                                                                                                                                                   |
| `lib/jsrunner`    | JS engine abstraction (Goja or native WASM). Used by d2latex.                                                                                                                              |
| `lib/urlenc`      | URL-safe D2 script encoding (flate + base64).                                                                                                                                              |
| `lib/log`         | Context-based structured logging (slog wrapper).                                                                                                                                           |
| `lib/env`         | Environment variable helpers: `Test()`, `Dev()`, `Debug()`, `Timeout()`.                                                                                                                   |
| `lib/syncmap`     | Generic type-safe `sync.Map` wrapper.                                                                                                                                                      |
| `lib/version`     | Version string (currently v0.7.1-HEAD).                                                                                                                                                    |
| `lib/time`        | `HumanDate()`, `WithTimeout()` (respects D2_TIMEOUT env).                                                                                                                                  |
| `lib/background`  | `Repeat()` — background task with ticker.                                                                                                                                                  |

### Other

- **d2chaos/** — Fuzz testing. `GenDSL(maxi)` generates random valid D2 scripts
  (25% new nodes, 75% edges).
- **d2js/** — WASM build for browsers. `d2js/d2wasm/` exposes 62+ APIs (Compile,
  Render, GetCompletions, etc.). `d2js/js/` has npm package with TypeScript
  types.
- **e2etests/** — 7000+ test files. `runa()` helper runs test cases and compares
  golden outputs. `report/main.go` generates visual HTML diff report.

## Fork Conventions

This is a **fork** of the upstream D2 repository. All fork-specific changes must
follow these rules:

### Change Marking

- All fork modifications in existing files MUST be marked with `// [FORK]`
  comments explaining what was changed and why
- New files/packages added by the fork MUST have a header comment:
  `// [FORK] This file is added by the fork for <feature>.`
- This makes fork changes grep-able: `grep -r "\[FORK\]" . --include="*.go"`

### Commit Convention

- Every commit MUST use the `[FORK:task-name]` prefix, where `task-name` matches
  the openspec change directory name (e.g., `add-grid-edge-routing`):
  `[FORK:add-grid-edge-routing] Fix float comparisons in routing graph`
- For commits not tied to a specific task, use plain `[FORK]`:
  `[FORK] Fix typo in README`
- The changelog script groups commits by task and filters WIP/meta (changelog
  updates, openspec docs). Keep commit messages meaningful — they become the
  changelog entries.
- Co-authored-by line is still required

### Upstream Merge Strategy

- Prefer creating **new packages/files** over modifying existing ones to
  minimize merge conflicts
- When modifying existing files, keep changes minimal and clearly bounded with
  `[FORK]` markers
- Import new fork packages into existing code rather than inlining logic
- Periodically check upstream for conflicts:
  `git diff upstream/master -- <modified-files>`

### Fork Goals

- Maximize parameterization of layout engines (expose all ELK/Dagre options that
  affect rendering)
- Improve edge routing quality (especially in grid diagrams)
- Keep the fork always mergeable with upstream updates

### Fork Packages

- `d2layouts/d2wueortho/` — Orthogonal graph drawing engine (Hegemann & Wolff 2023)

## Known Pitfalls

Problems encountered during fork development that are likely to recur.

### Edge routes produce NaN in SVG

**Symptom**: SVG paths contain `NaN` values, edges disappear or render incorrectly.
**Root cause**: `edge.TraceToShape()` clips route endpoints to shape boundaries.
When a port is already exactly on the boundary, TraceToShape produces a zero-length
segment. The d2svg bezier renderer then divides by zero when computing curve normals.
**Fix**: For edges with pre-computed port positions (e.g., from d2wueortho), skip
TraceToShape entirely. Set `edge.Route` directly and let the SVG renderer handle
arrowhead offsets via its own `arrowheadAdjustment()`.

### Floating-point coordinate noise from routing graph

**Symptom**: Routes that should be straight lines (e.g., vertical edge between
same-column nodes) render with tiny curves or kinks at every intermediate point.
**Root cause**: The routing graph produces intermediate points with floating-point
noise (e.g., X values 278.166, 278.169, 278.170 instead of a single value). The
d2svg bezier renderer treats each sub-pixel difference as a real corner and draws
a curve. `simplifyRoute` with exact equality (`==`) does not remove near-collinear
points.
**Fix**: Use tolerance-based comparison (0.5px) in `simplifyRoute`, and snap
near-aligned coordinates to eliminate floating-point jitter before SVG rendering.

### d2svg arrowheadAdjustment shifts route endpoints

**Symptom**: Arrow tips enter shapes at slightly wrong positions (~2-4px offset).
**Root cause**: `d2svg.arrowheadAdjustment()` in `d2svg.go:812` shifts the first
and last route points inward by `(edgeStrokeWidth + shapeStrokeWidth)/2` plus
`edgeStrokeWidth` for arrowheads. This is intentional (creates space for the SVG
arrowhead marker) but means the visual endpoint differs from `edge.Route[last]`.
**Impact**: Cannot be "fixed" from the router side — this is d2svg behavior. Port
positions should be placed exactly on shape boundaries; d2svg will handle the visual
offset. Don't try to pre-compensate.

### ELK cannot be used as a standalone edge router

**Symptom**: Edge routes cross through nodes regardless of INTERACTIVE strategy
settings.
**Root cause**: ELK's layered (Sugiyama) algorithm assigns nodes to discrete layers
and routes edges between layers. It fundamentally cannot preserve arbitrary fixed
node positions — it MUST rearrange nodes to satisfy layer constraints.
**Lesson**: ELK is a node layout + edge routing engine, not a standalone edge router.
For routing edges with pre-positioned nodes, use a dedicated algorithm (e.g.,
channel-based routing from Hegemann & Wolff 2023).

### Grid edge router: face selection determines visual quality

**Context**: `d2layouts/d2wueortho/gridroute.go` — the L/Z-Router for standalone
wueortho layout.

**Core principle**: Face selection (which side of a shape an edge enters/exits)
is the #1 factor in visual quality. Port positions, L-route orientation, and
crossing detection are secondary. Get face selection right first.

**Face selection architecture** (two-pass):
- **Pass 1** (mandatory): Same-row → RIGHT/LEFT. Same-col → BOTTOM/TOP.
  Strictly dominant diagonal (|dc|>|dr| or |dr|>|dc|) → dominant axis faces.
- **Pass 2** (flex): Equal diagonals (|dc|==|dr|) use **independent per-endpoint
  face selection**. Each endpoint picks the face with the lowest current load
  among its two candidate faces (vertical and horizontal toward the peer).
  This creates **mixed face pairs** (e.g., src exits BOTTOM, dst enters LEFT)
  which push L-route bends to layout corners.
- **Tiebreaker**: when loads are equal, prefer **vertical** faces (`<=` not `<`).
  This matters because mandatory same-col edges already occupy vertical faces,
  and the tiebreaker determines how remaining edges distribute. Vertical
  preference naturally cascades: first flex edges take vertical, which increases
  vertical load, causing later edges to pick horizontal.

**L-route orientation must match srcFace direction**:
- When srcFace is TOP/BOTTOM (vertical exit): try vertical-first L-route
  (bend at `(src.X, dst.Y)`) before horizontal-first.
- When srcFace is LEFT/RIGHT (horizontal exit): try horizontal-first L-route
  (bend at `(dst.X, src.Y)`) before vertical-first.
- Without this, L-routes bend in the wrong direction — creating bends in the
  center of the layout instead of at the exterior.

**Port alignment for straight edges**: When two nodes in the same column have a
straight vertical edge, align both ports to the face with FEWER ports (it's
centered). The face with MORE ports adjusts to match. NOT the other way around —
aligning to the multi-port face pulls centered single-port faces off-center.

**Debugging approach**: Don't make random changes to port spread formulas or face
selection heuristics. Instead: (1) extract exact SVG coordinates with a Python
script, (2) compute per-edge face assignments and port offsets from face center,
(3) trace through the algorithm with actual numbers to understand WHY, (4) make
a targeted fix. The user explicitly said: "ты понимаешь все цифры, обдумай их."

**Known limitation**: Non-equal diagonals (|dc|≠|dr|) with large horizontal
distance produce L-routes with long bridge segments through empty space (see
task 4.16). This needs either extended mixed face selection or a route quality
heuristic.

### Grid edge router: SVG path structure

D2's SVG renderer (`d2svg`) converts orthogonal routes (sequences of H/V points)
into smooth SVG paths using `S` (smooth cubic Bezier) commands at bend points.
When parsing SVG paths to analyze routes:
- `M x y` = move to (start point)
- `L x y` = line to (straight segment)
- `S cx cy x y` = smooth curve (cx,cy is control point, x,y is endpoint)
- A 3-point orthogonal L-route `[A, bend, B]` becomes `M A L near-bend S bend B`
  — this appears as 4 SVG points but is logically 1 bend.
- Edge labels have no group IDs in SVG. To identify which edge is which, match
  start/end coordinates against known node positions.

## Playwright / Browser Preview

When using the Playwright MCP to view SVGs or HTML files:
- `file://` protocol is BLOCKED. Never use `file:///path/to/file.svg`.
- Instead, use the `/d2-diagrams` skill which renders via a local D2 server at
  `localhost:3000`.
- For ad-hoc file viewing, serve the directory first:
  `python3 -m http.server 8765 --directory /path/to/dir` then navigate to
  `http://localhost:8765/filename.svg`.

## Requires

- Go 1.25+
- Signed commits for PRs
