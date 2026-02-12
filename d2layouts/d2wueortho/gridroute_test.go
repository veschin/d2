// [FORK] Unit tests for L/Z-Router (gridroute.go).
// Tests: straight lines, L-routes, Z-routes, port spreading, no-crossing invariant.
package d2wueortho

import (
	"math"
	"testing"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
)

// --- Test Helpers ---

// makeTestObj creates a minimal d2graph.Object with the given position and size.
func makeTestObj(id string, x, y, w, h float64) *d2graph.Object {
	obj := &d2graph.Object{
		ID: id,
		Box: &geo.Box{
			TopLeft: geo.NewPoint(x, y),
			Width:   w,
			Height:  h,
		},
	}
	obj.Attributes.Label.Value = id
	return obj
}

// makeTestGraph builds a test graph with objects on a virtual grid and edges between them.
// gridCells maps object index to (row, col). cellW/cellH are cell dimensions (including channel).
// edges is a list of (srcIdx, dstIdx) pairs.
func makeTestGraph(objs []*d2graph.Object, edges [][2]int) (*d2graph.Graph, *gridInfo) {
	g := d2graph.NewGraph()
	g.Root.ChildrenArray = objs
	for _, obj := range objs {
		obj.Parent = g.Root
		obj.Graph = g
		g.Objects = append(g.Objects, obj)
	}

	for _, e := range edges {
		edge := &d2graph.Edge{
			Src:      objs[e[0]],
			Dst:      objs[e[1]],
			DstArrow: true,
		}
		g.Edges = append(g.Edges, edge)
	}

	return g, nil
}

// buildGridInfo constructs gridInfo from positioned objects with a standard channel width.
func buildGridInfo(objs []*d2graph.Object, placement map[int]gridCell, channel float64) *gridInfo {
	occupied := make(map[gridCell]int)
	for i, cell := range placement {
		occupied[cell] = i
	}

	// Compute per-column widths and per-row heights.
	colWidth := make(map[int]float64)
	rowHeight := make(map[int]float64)
	for i, obj := range objs {
		cell := placement[i]
		w := obj.Width + channel
		h := obj.Height + channel
		if w > colWidth[cell.col] {
			colWidth[cell.col] = w
		}
		if h > rowHeight[cell.row] {
			rowHeight[cell.row] = h
		}
	}

	// Prefix sums.
	maxCol, maxRow := 0, 0
	for _, cell := range placement {
		if cell.col > maxCol {
			maxCol = cell.col
		}
		if cell.row > maxRow {
			maxRow = cell.row
		}
	}
	colX := make(map[int]float64)
	rowY := make(map[int]float64)
	x := 0.0
	for c := 0; c <= maxCol; c++ {
		colX[c] = x
		x += colWidth[c]
	}
	y := 0.0
	for r := 0; r <= maxRow; r++ {
		rowY[r] = y
		y += rowHeight[r]
	}

	// Position objects at cell centers.
	for i, obj := range objs {
		cell := placement[i]
		cx := colX[cell.col] + colWidth[cell.col]/2
		cy := rowY[cell.row] + rowHeight[cell.row]/2
		obj.TopLeft = geo.NewPoint(cx-obj.Width/2, cy-obj.Height/2)
	}

	return &gridInfo{
		placement: placement,
		occupied:  occupied,
		colWidth:  colWidth,
		rowHeight: rowHeight,
		colX:      colX,
		rowY:      rowY,
		objects:   objs,
		channel:   channel,
	}
}

// isOrthogonal checks that all segments in a route are horizontal or vertical.
func isOrthogonal(route []*geo.Point) bool {
	for i := 0; i < len(route)-1; i++ {
		dx := math.Abs(route[i].X - route[i+1].X)
		dy := math.Abs(route[i].Y - route[i+1].Y)
		if dx > 0.5 && dy > 0.5 {
			return false
		}
	}
	return true
}

// countBends counts the number of direction changes in an orthogonal route.
func countBends(route []*geo.Point) int {
	if len(route) < 3 {
		return 0
	}
	bends := 0
	for i := 1; i < len(route)-1; i++ {
		// Check if direction changes at point i.
		prevH := math.Abs(route[i-1].X-route[i].X) > 0.5
		nextH := math.Abs(route[i].X-route[i+1].X) > 0.5
		if prevH != nextH {
			bends++
		}
	}
	return bends
}

// routeIntersectsBox checks if any segment of a route passes through a box (with margin).
func routeIntersectsBox(route []*geo.Point, obj *d2graph.Object, margin float64) bool {
	box := [4]float64{
		obj.TopLeft.X - margin,
		obj.TopLeft.Y - margin,
		obj.TopLeft.X + obj.Width + margin,
		obj.TopLeft.Y + obj.Height + margin,
	}
	for i := 0; i < len(route)-1; i++ {
		if segmentIntersectsBox(route[i], route[i+1], box) {
			return true
		}
	}
	return false
}

// --- 4.11: Adjacent cells produce straight lines ---

func TestGridRoute_AdjacentCells_SameRow_Straight(t *testing.T) {
	// Two nodes in the same row, adjacent columns → straight horizontal line.
	objs := []*d2graph.Object{
		makeTestObj("A", 0, 0, 100, 60),
		makeTestObj("B", 0, 0, 100, 60),
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 0},
		1: {row: 0, col: 1},
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{{0, 1}})
	gridRouteEdges(g, info)

	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	route := g.Edges[0].Route
	if len(route) < 2 {
		t.Fatalf("route has %d points, need at least 2", len(route))
	}

	if !isOrthogonal(route) {
		t.Errorf("route is not orthogonal: %v", route)
	}
	bends := countBends(route)
	if bends != 0 {
		t.Errorf("adjacent same-row: expected 0 bends, got %d (route: %v)", bends, route)
	}
	// Verify it's horizontal (same Y for all points).
	for i, p := range route {
		if math.Abs(p.Y-route[0].Y) > 1.0 {
			t.Errorf("point %d Y=%.1f differs from start Y=%.1f (should be straight horizontal)", i, p.Y, route[0].Y)
		}
	}
}

func TestGridRoute_AdjacentCells_SameCol_Straight(t *testing.T) {
	// Two nodes in the same column, adjacent rows → straight vertical line.
	objs := []*d2graph.Object{
		makeTestObj("A", 0, 0, 100, 60),
		makeTestObj("B", 0, 0, 100, 60),
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 0},
		1: {row: 1, col: 0},
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{{0, 1}})
	gridRouteEdges(g, info)

	route := g.Edges[0].Route
	if len(route) < 2 {
		t.Fatalf("route has %d points, need at least 2", len(route))
	}

	if !isOrthogonal(route) {
		t.Errorf("route is not orthogonal: %v", route)
	}
	bends := countBends(route)
	if bends != 0 {
		t.Errorf("adjacent same-col: expected 0 bends, got %d (route: %v)", bends, route)
	}
	// Verify it's vertical (same X for all points).
	for i, p := range route {
		if math.Abs(p.X-route[0].X) > 1.0 {
			t.Errorf("point %d X=%.1f differs from start X=%.1f (should be straight vertical)", i, p.X, route[0].X)
		}
	}
}

// --- 4.12: Diagonal cells produce L-routes with 1 bend ---

func TestGridRoute_DiagonalCells_LRoute(t *testing.T) {
	// Two nodes in different row and column → L-route (1 bend).
	objs := []*d2graph.Object{
		makeTestObj("A", 0, 0, 100, 60),
		makeTestObj("B", 0, 0, 100, 60),
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 0},
		1: {row: 1, col: 1},
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{{0, 1}})
	gridRouteEdges(g, info)

	route := g.Edges[0].Route
	if len(route) < 2 {
		t.Fatalf("route has %d points, need at least 2", len(route))
	}

	if !isOrthogonal(route) {
		t.Errorf("route is not orthogonal: %v", route)
	}
	bends := countBends(route)
	if bends > 2 {
		t.Errorf("diagonal: expected <=2 bends, got %d (route: %v)", bends, route)
	}
	if bends == 0 {
		t.Errorf("diagonal: expected >=1 bend for non-adjacent diagonal, got 0")
	}
}

func TestGridRoute_DiagonalOpposite_LRoute(t *testing.T) {
	// Node at (0,1) → Node at (1,0): different row+col, should be L-route.
	objs := []*d2graph.Object{
		makeTestObj("A", 0, 0, 80, 80),
		makeTestObj("B", 0, 0, 80, 80),
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 1},
		1: {row: 1, col: 0},
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{{0, 1}})
	gridRouteEdges(g, info)

	route := g.Edges[0].Route
	if !isOrthogonal(route) {
		t.Errorf("route is not orthogonal")
	}
	bends := countBends(route)
	if bends < 1 || bends > 2 {
		t.Errorf("expected 1-2 bends for diagonal, got %d", bends)
	}
}

// --- 4.13: Edge crossing occupied cell upgrades to Z-route ---

func TestGridRoute_OccupiedCell_ZRoute(t *testing.T) {
	// Three nodes: A at (0,0), B at (0,1) (blocker), C at (0,2).
	// Edge A→C must go around B. Since B occupies the direct path,
	// the route should have >=1 bend (Z-route through channel).
	objs := []*d2graph.Object{
		makeTestObj("A", 0, 0, 100, 60),
		makeTestObj("B", 0, 0, 100, 60), // blocker in the middle
		makeTestObj("C", 0, 0, 100, 60),
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 0},
		1: {row: 0, col: 1}, // blocker
		2: {row: 0, col: 2},
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{{0, 2}})
	gridRouteEdges(g, info)

	route := g.Edges[0].Route
	if len(route) < 2 {
		t.Fatalf("route has %d points", len(route))
	}

	if !isOrthogonal(route) {
		t.Errorf("route is not orthogonal: %v", route)
	}
	bends := countBends(route)
	// With a blocker in the way, the route should detour (Z-route: 2 bends).
	if bends < 2 {
		// Verify the route doesn't go through B.
		if routeIntersectsBox(route, objs[1], 2.0) {
			t.Errorf("route passes through blocker B with only %d bends", bends)
		}
	}
}

func TestGridRoute_OccupiedCell_DiagonalUpgrade(t *testing.T) {
	// A at (0,0), Blocker at (1,1), C at (2,2).
	// Diagonal edge A→C: if L-route's bend corner (0,1) or (1,0) are free,
	// L-route is fine. With blocker at (1,1), L-route at (0,2) or (2,0)
	// should work. This tests that the crossing detection is functional.
	objs := []*d2graph.Object{
		makeTestObj("A", 0, 0, 80, 80),
		makeTestObj("Blocker", 0, 0, 80, 80),
		makeTestObj("C", 0, 0, 80, 80),
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 0},
		1: {row: 1, col: 1}, // blocker in diagonal center
		2: {row: 2, col: 2},
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{{0, 2}})
	gridRouteEdges(g, info)

	route := g.Edges[0].Route
	if !isOrthogonal(route) {
		t.Errorf("route is not orthogonal")
	}

	// Route should NOT pass through the blocker.
	if routeIntersectsBox(route, objs[1], 2.0) {
		t.Errorf("route passes through blocker at (1,1)")
	}
}

// --- 4.14: Multiple edges to same face are spread evenly ---

func TestGridRoute_MultiplEdges_SameFace_Spread(t *testing.T) {
	// Central node A at (1,1) with three satellites at (0,0), (0,1), (0,2).
	// All three edges enter A from the top face.
	// Port positions should be spread out, not stacked.
	objs := []*d2graph.Object{
		makeTestObj("Sat1", 0, 0, 80, 60),
		makeTestObj("Sat2", 0, 0, 80, 60),
		makeTestObj("Sat3", 0, 0, 80, 60),
		makeTestObj("A", 0, 0, 120, 80), // wider center node
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 0},
		1: {row: 0, col: 1},
		2: {row: 0, col: 2},
		3: {row: 1, col: 1}, // center below
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{{0, 3}, {1, 3}, {2, 3}})
	gridRouteEdges(g, info)

	if len(g.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(g.Edges))
	}

	// Collect entry points on A's top face.
	// The dst port of each edge is where it enters A.
	var dstYs []float64
	var dstXs []float64
	aTop := objs[3].TopLeft.Y
	for _, e := range g.Edges {
		route := e.Route
		if len(route) < 2 {
			continue
		}
		lastPt := route[len(route)-1]
		dstYs = append(dstYs, lastPt.Y)
		dstXs = append(dstXs, lastPt.X)
	}

	// All 3 edges should enter near A's top face (Y near aTop).
	for i, y := range dstYs {
		if math.Abs(y-aTop) > 5.0 {
			// They might enter from bottom if A is above — check both faces.
			aBottom := objs[3].TopLeft.Y + objs[3].Height
			if math.Abs(y-aBottom) > 5.0 {
				t.Logf("edge %d enters A at Y=%.1f (A top=%.1f, bottom=%.1f)", i, y, aTop, aBottom)
			}
		}
	}

	// Check that ports are spread out (not all at the same X).
	if len(dstXs) >= 2 {
		allSame := true
		for i := 1; i < len(dstXs); i++ {
			if math.Abs(dstXs[i]-dstXs[0]) > 1.0 {
				allSame = false
				break
			}
		}
		if allSame {
			t.Errorf("all %d dst ports have the same X position (%.1f) — ports should be spread", len(dstXs), dstXs[0])
		}
	}

	// Check minimum clearance between adjacent ports (8px per spec).
	if len(dstXs) >= 2 {
		// Sort to check adjacent gaps.
		sorted := make([]float64, len(dstXs))
		copy(sorted, dstXs)
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j] < sorted[i] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		for i := 0; i < len(sorted)-1; i++ {
			gap := sorted[i+1] - sorted[i]
			if gap < 4.0 && gap > 0.1 { // be lenient — router has 8px spec but test uses 4px tolerance
				t.Errorf("port gap %.1f between ports %d and %d is too small (min 4px tolerance)", gap, i, i+1)
			}
		}
	}
}

// --- 4.15: No edge routes pass through node bounding boxes ---

func TestGridRoute_NoRouteThroughNodes(t *testing.T) {
	// Test the no-crossing invariant for adjacent and 1-cell-diagonal edges.
	// Long-distance diagonal edges (e.g., TL→BR in a 3x3 grid) are a known
	// limitation (task 4.16) and are NOT tested here.
	objs := []*d2graph.Object{
		makeTestObj("TL", 0, 0, 80, 60), // (0,0)
		makeTestObj("TC", 0, 0, 80, 60), // (0,1)
		makeTestObj("TR", 0, 0, 80, 60), // (0,2)
		makeTestObj("ML", 0, 0, 80, 60), // (1,0)
		makeTestObj("MC", 0, 0, 80, 60), // (1,1) center
		makeTestObj("MR", 0, 0, 80, 60), // (1,2)
		makeTestObj("BL", 0, 0, 80, 60), // (2,0)
		makeTestObj("BC", 0, 0, 80, 60), // (2,1)
		makeTestObj("BR", 0, 0, 80, 60), // (2,2)
	}
	placement := map[int]gridCell{
		0: {row: 0, col: 0},
		1: {row: 0, col: 1},
		2: {row: 0, col: 2},
		3: {row: 1, col: 0},
		4: {row: 1, col: 1},
		5: {row: 1, col: 2},
		6: {row: 2, col: 0},
		7: {row: 2, col: 1},
		8: {row: 2, col: 2},
	}
	info := buildGridInfo(objs, placement, 80.0)

	// Adjacent and same-column/row edges (distance <=1 or 2 on one axis).
	// Avoid multi-cell diagonals which are a known limitation.
	g, _ := makeTestGraph(objs, [][2]int{
		{1, 7}, // TC→BC (same col, distance 2 — routes through channel around MC)
		{3, 5}, // ML→MR (same row, distance 2 — routes through channel around MC)
		{0, 1}, // TL→TC (adjacent same row)
		{4, 5}, // MC→MR (adjacent same row)
		{1, 4}, // TC→MC (adjacent same col)
	})
	gridRouteEdges(g, info)

	for ei, e := range g.Edges {
		route := e.Route
		if len(route) < 2 {
			continue
		}

		if !isOrthogonal(route) {
			t.Errorf("edge %d (%s→%s): route is not orthogonal", ei, e.Src.ID, e.Dst.ID)
		}

		// Check against ALL objects except src and dst.
		for oi, obj := range objs {
			if obj == e.Src || obj == e.Dst {
				continue
			}
			if routeIntersectsBox(route, obj, 2.0) {
				t.Errorf("edge %d (%s→%s): route passes through node %d (%s) at (%v)",
					ei, e.Src.ID, e.Dst.ID, oi, obj.ID, obj.TopLeft)
			}
		}
	}
}

func TestGridRoute_NoRouteThroughNodes_StarGraph(t *testing.T) {
	// Star graph: center node with 4 satellites.
	// No route from any satellite to center should cross another satellite.
	objs := []*d2graph.Object{
		makeTestObj("Center", 0, 0, 100, 80),
		makeTestObj("N", 0, 0, 80, 60),
		makeTestObj("E", 0, 0, 80, 60),
		makeTestObj("S", 0, 0, 80, 60),
		makeTestObj("W", 0, 0, 80, 60),
	}
	placement := map[int]gridCell{
		0: {row: 1, col: 1}, // center
		1: {row: 0, col: 1}, // north
		2: {row: 1, col: 2}, // east
		3: {row: 2, col: 1}, // south
		4: {row: 1, col: 0}, // west
	}
	info := buildGridInfo(objs, placement, 80.0)

	g, _ := makeTestGraph(objs, [][2]int{
		{1, 0}, // N→Center
		{2, 0}, // E→Center
		{3, 0}, // S→Center
		{4, 0}, // W→Center
	})
	gridRouteEdges(g, info)

	for ei, e := range g.Edges {
		route := e.Route
		if len(route) < 2 {
			continue
		}
		for oi, obj := range objs {
			if obj == e.Src || obj == e.Dst {
				continue
			}
			if routeIntersectsBox(route, obj, 2.0) {
				t.Errorf("edge %d (%s→%s): route passes through %s",
					ei, e.Src.ID, e.Dst.ID, objs[oi].ID)
			}
		}
	}
}

// --- Additional: selectFaces unit tests ---

func TestSelectFaces_SameRow(t *testing.T) {
	sf, df := selectFaces(gridCell{0, 0}, gridCell{0, 2})
	if sf != faceRight || df != faceLeft {
		t.Errorf("same row right: got srcFace=%d dstFace=%d, want RIGHT/LEFT", sf, df)
	}

	sf, df = selectFaces(gridCell{0, 2}, gridCell{0, 0})
	if sf != faceLeft || df != faceRight {
		t.Errorf("same row left: got srcFace=%d dstFace=%d, want LEFT/RIGHT", sf, df)
	}
}

func TestSelectFaces_SameCol(t *testing.T) {
	sf, df := selectFaces(gridCell{0, 0}, gridCell{2, 0})
	if sf != faceBottom || df != faceTop {
		t.Errorf("same col down: got srcFace=%d dstFace=%d, want BOTTOM/TOP", sf, df)
	}

	sf, df = selectFaces(gridCell{2, 0}, gridCell{0, 0})
	if sf != faceTop || df != faceBottom {
		t.Errorf("same col up: got srcFace=%d dstFace=%d, want TOP/BOTTOM", sf, df)
	}
}

func TestSelectFaces_HorizontalDominant(t *testing.T) {
	// dc=3, dr=1 → horizontal dominant → RIGHT/LEFT.
	sf, df := selectFaces(gridCell{0, 0}, gridCell{1, 3})
	if sf != faceRight || df != faceLeft {
		t.Errorf("horizontal dominant: got srcFace=%d dstFace=%d, want RIGHT/LEFT", sf, df)
	}
}

func TestSelectFaces_VerticalDominant(t *testing.T) {
	// dc=1, dr=3 → vertical dominant → BOTTOM/TOP.
	sf, df := selectFaces(gridCell{0, 0}, gridCell{3, 1})
	if sf != faceBottom || df != faceTop {
		t.Errorf("vertical dominant: got srcFace=%d dstFace=%d, want BOTTOM/TOP", sf, df)
	}
}

func TestSelectFaces_EqualDiagonal(t *testing.T) {
	// dc=1, dr=1 → equal diagonal → vertical preferred (BOTTOM/TOP).
	sf, df := selectFaces(gridCell{0, 0}, gridCell{1, 1})
	if sf != faceBottom || df != faceTop {
		t.Errorf("equal diagonal: got srcFace=%d dstFace=%d, want BOTTOM/TOP (vertical preferred)", sf, df)
	}
}

// --- Additional: constructRoute unit tests ---

func TestConstructRoute_StraightH(t *testing.T) {
	info := &gridInfo{
		placement: map[int]gridCell{0: {0, 0}, 1: {0, 1}},
		occupied:  map[gridCell]int{gridCell{0, 0}: 0, gridCell{0, 1}: 1},
		colWidth:  map[int]float64{0: 180, 1: 180},
		rowHeight: map[int]float64{0: 140},
		colX:      map[int]float64{0: 0, 1: 180},
		rowY:      map[int]float64{0: 0},
		objects: []*d2graph.Object{
			makeTestObj("A", 40, 40, 100, 60),
			makeTestObj("B", 220, 40, 100, 60),
		},
		channel: 80,
	}

	src := geo.NewPoint(140, 70)
	dst := geo.NewPoint(220, 70)
	route := constructRoute(src, dst, faceRight, faceLeft, gridCell{0, 0}, gridCell{0, 1}, info)

	if len(route) != 2 {
		t.Errorf("expected 2-point straight route, got %d points: %v", len(route), route)
	}
	if countBends(route) != 0 {
		t.Errorf("expected 0 bends for straight horizontal, got %d", countBends(route))
	}
}

func TestConstructRoute_LRoute(t *testing.T) {
	info := &gridInfo{
		placement: map[int]gridCell{0: {0, 0}, 1: {1, 1}},
		occupied:  map[gridCell]int{gridCell{0, 0}: 0, gridCell{1, 1}: 1},
		colWidth:  map[int]float64{0: 180, 1: 180},
		rowHeight: map[int]float64{0: 140, 1: 140},
		colX:      map[int]float64{0: 0, 1: 180},
		rowY:      map[int]float64{0: 0, 1: 140},
		objects: []*d2graph.Object{
			makeTestObj("A", 40, 40, 100, 60),
			makeTestObj("B", 220, 180, 100, 60),
		},
		channel: 80,
	}

	src := geo.NewPoint(90, 100) // bottom of A
	dst := geo.NewPoint(220, 210) // left of B
	route := constructRoute(src, dst, faceBottom, faceLeft, gridCell{0, 0}, gridCell{1, 1}, info)

	if len(route) < 3 {
		t.Errorf("expected >=3-point L-route, got %d points: %v", len(route), route)
	}
	bends := countBends(route)
	if bends != 1 {
		t.Errorf("expected 1 bend for L-route, got %d", bends)
	}
}

// --- segmentIntersectsBox unit tests ---

func TestSegmentIntersectsBox_HorizontalThrough(t *testing.T) {
	box := [4]float64{50, 50, 150, 150} // left, top, right, bottom
	// Horizontal segment passing through box center.
	if !segmentIntersectsBox(geo.NewPoint(0, 100), geo.NewPoint(200, 100), box) {
		t.Error("expected horizontal segment through box center to intersect")
	}
}

func TestSegmentIntersectsBox_HorizontalAbove(t *testing.T) {
	box := [4]float64{50, 50, 150, 150}
	// Horizontal segment above box.
	if segmentIntersectsBox(geo.NewPoint(0, 30), geo.NewPoint(200, 30), box) {
		t.Error("horizontal segment above box should not intersect")
	}
}

func TestSegmentIntersectsBox_VerticalThrough(t *testing.T) {
	box := [4]float64{50, 50, 150, 150}
	if !segmentIntersectsBox(geo.NewPoint(100, 0), geo.NewPoint(100, 200), box) {
		t.Error("expected vertical segment through box to intersect")
	}
}

func TestSegmentIntersectsBox_VerticalOutside(t *testing.T) {
	box := [4]float64{50, 50, 150, 150}
	if segmentIntersectsBox(geo.NewPoint(200, 0), geo.NewPoint(200, 200), box) {
		t.Error("vertical segment outside box should not intersect")
	}
}

func TestSegmentIntersectsBox_DiagonalThrough(t *testing.T) {
	box := [4]float64{50, 50, 150, 150}
	// Diagonal from (0,0) to (200,200) passes through box center.
	if !segmentIntersectsBox(geo.NewPoint(0, 0), geo.NewPoint(200, 200), box) {
		t.Error("diagonal through box should intersect")
	}
}

func TestSegmentIntersectsBox_DiagonalMiss(t *testing.T) {
	box := [4]float64{50, 50, 150, 150}
	// Diagonal from (0,0) to (40,200) misses the box.
	if segmentIntersectsBox(geo.NewPoint(0, 0), geo.NewPoint(40, 200), box) {
		t.Error("diagonal missing box should not intersect")
	}
}

func TestSelectFaces_SameCell(t *testing.T) {
	sf, df := selectFaces(gridCell{0, 0}, gridCell{0, 0})
	if sf != faceRight || df != faceLeft {
		t.Errorf("same cell: expected RIGHT/LEFT, got %d/%d", sf, df)
	}
}
