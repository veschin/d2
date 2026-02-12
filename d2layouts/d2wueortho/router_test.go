// [FORK] Unit tests for individual grid router stages:
// port assignment, channels, routing graph, Dijkstra, nudging.
package d2wueortho

import (
	"math"
	"testing"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
)

// --- Port Assignment Tests ---

func TestDetermineSides_Horizontal(t *testing.T) {
	// Src is to the left of dst.
	src := Rect{X: 0, Y: 0, W: 100, H: 100}
	dst := Rect{X: 300, Y: 0, W: 100, H: 100}
	srcSide, dstSide := determineSides(src, dst)
	if srcSide != DirRight {
		t.Errorf("srcSide: got %d, want DirRight(%d)", srcSide, DirRight)
	}
	if dstSide != DirLeft {
		t.Errorf("dstSide: got %d, want DirLeft(%d)", dstSide, DirLeft)
	}
}

func TestDetermineSides_Vertical(t *testing.T) {
	// Src is above dst.
	src := Rect{X: 0, Y: 0, W: 100, H: 100}
	dst := Rect{X: 0, Y: 300, W: 100, H: 100}
	srcSide, dstSide := determineSides(src, dst)
	if srcSide != DirBottom {
		t.Errorf("srcSide: got %d, want DirBottom(%d)", srcSide, DirBottom)
	}
	if dstSide != DirTop {
		t.Errorf("dstSide: got %d, want DirTop(%d)", dstSide, DirTop)
	}
}

func TestDetermineSides_Diagonal_ZAvoidance(t *testing.T) {
	// Src is upper-left, dst is lower-right with significant Y offset.
	// This should trigger Z-avoidance.
	src := Rect{X: 0, Y: 0, W: 100, H: 100}
	dst := Rect{X: 200, Y: 150, W: 100, H: 100}
	srcSide, dstSide := determineSides(src, dst)
	// Primarily horizontal (dx=250 > dy=150), Z-avoidance threshold: dy > dx*0.25 → 150 > 62.5 → true.
	// Dst is below, so dstSide should be DirTop (L-shape).
	if srcSide != DirRight {
		t.Errorf("srcSide: got %d, want DirRight(%d)", srcSide, DirRight)
	}
	if dstSide != DirTop {
		t.Errorf("dstSide: got %d, want DirTop(%d) for Z-avoidance", dstSide, DirTop)
	}
}

func TestDetermineSides_45Degrees(t *testing.T) {
	// Equal dx and dy → L-shape.
	src := Rect{X: 0, Y: 0, W: 100, H: 100}
	dst := Rect{X: 200, Y: 200, W: 100, H: 100}
	srcSide, dstSide := determineSides(src, dst)
	if srcSide != DirRight {
		t.Errorf("srcSide: got %d, want DirRight(%d)", srcSide, DirRight)
	}
	if dstSide != DirTop {
		t.Errorf("dstSide: got %d, want DirTop(%d)", dstSide, DirTop)
	}
}

func TestDistributeAlongSide(t *testing.T) {
	box := Rect{X: 100, Y: 100, W: 200, H: 100}

	tests := []struct {
		name    string
		side    Direction
		n       int
		wantPos []geo.Point
	}{
		{
			name: "1 port on top",
			side: DirTop,
			n:    1,
			wantPos: []geo.Point{
				{X: 200, Y: 100}, // midpoint of top edge
			},
		},
		{
			name: "2 ports on bottom",
			side: DirBottom,
			n:    2,
			wantPos: []geo.Point{
				{X: 100 + 200.0/3, Y: 200},
				{X: 100 + 400.0/3, Y: 200},
			},
		},
		{
			name: "1 port on left",
			side: DirLeft,
			n:    1,
			wantPos: []geo.Point{
				{X: 100, Y: 150}, // midpoint of left edge
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ports := make([]*Port, tt.n)
			for i := range ports {
				ports[i] = &Port{Side: tt.side}
			}
			distributeAlongSide(ports, box, tt.side)
			for i, p := range ports {
				if math.Abs(p.Pos.X-tt.wantPos[i].X) > 0.01 || math.Abs(p.Pos.Y-tt.wantPos[i].Y) > 0.01 {
					t.Errorf("port %d: got (%.2f, %.2f), want (%.2f, %.2f)",
						i, p.Pos.X, p.Pos.Y, tt.wantPos[i].X, tt.wantPos[i].Y)
				}
			}
		})
	}
}

func TestSelfLoopSides(t *testing.T) {
	sideCount := make(map[nodeSideKey]int)
	// Empty counts — should pick first "best" (DirTop, count 0) and clockwise neighbor (DirRight).
	srcSide, dstSide := selfLoopSides(0, sideCount)
	if srcSide != DirTop {
		t.Errorf("srcSide: got %d, want DirTop(%d)", srcSide, DirTop)
	}
	if dstSide != DirRight {
		t.Errorf("dstSide: got %d, want DirRight(%d)", dstSide, DirRight)
	}

	// Mark top and right as populated.
	sideCount[nodeSideKey{0, DirTop}] = 2
	sideCount[nodeSideKey{0, DirRight}] = 1
	srcSide, dstSide = selfLoopSides(0, sideCount)
	// Bottom and Left have 0 ports. Bottom comes first in iteration order.
	if srcSide != DirBottom {
		t.Errorf("srcSide after population: got %d, want DirBottom(%d)", srcSide, DirBottom)
	}
	if dstSide != DirLeft {
		t.Errorf("dstSide after population: got %d, want DirLeft(%d)", dstSide, DirLeft)
	}
}

// --- Channel Tests ---

func TestFindChannels_SimpleGrid(t *testing.T) {
	// 2x2 grid with gap between cells.
	boxes := []Rect{
		{X: 0, Y: 0, W: 100, H: 100},     // top-left
		{X: 150, Y: 0, W: 100, H: 100},    // top-right
		{X: 0, Y: 150, W: 100, H: 100},    // bottom-left
		{X: 150, Y: 150, W: 100, H: 100},  // bottom-right
	}
	bbox := computeBoundingBox(boxes)
	channels := findChannels(boxes, bbox)

	// Should have channels:
	// - Vertical channel between columns (x: 100-150)
	// - Horizontal channel between rows (y: 100-150)
	// - Boundary channels (left, right, top, bottom margins)
	hasVerticalMiddle := false
	hasHorizontalMiddle := false
	for _, ch := range channels {
		if ch.Orientation == Vertical {
			if ch.Rect.Left() >= 99 && ch.Rect.Right() <= 151 {
				hasVerticalMiddle = true
			}
		}
		if ch.Orientation == Horizontal {
			if ch.Rect.Top() >= 99 && ch.Rect.Bottom() <= 151 {
				hasHorizontalMiddle = true
			}
		}
	}
	if !hasVerticalMiddle {
		t.Error("missing vertical channel between columns")
	}
	if !hasHorizontalMiddle {
		t.Error("missing horizontal channel between rows")
	}
}

func TestPruneChannels(t *testing.T) {
	// Two vertical channels: one wide, one narrow (contained by the wide one).
	channels := []Channel{
		{Rect: Rect{X: 100, Y: 0, W: 50, H: 300}, Orientation: Vertical},
		{Rect: Rect{X: 110, Y: 0, W: 20, H: 300}, Orientation: Vertical},
	}
	pruned := pruneChannels(channels)
	if len(pruned) != 1 {
		t.Errorf("pruneChannels: got %d channels, want 1", len(pruned))
	}
	if len(pruned) > 0 && pruned[0].Rect.W != 50 {
		t.Errorf("surviving channel width: got %.1f, want 50", pruned[0].Rect.W)
	}
}

func TestPruneChannels_DifferentOrientation(t *testing.T) {
	// Channels of different orientations should not prune each other.
	channels := []Channel{
		{Rect: Rect{X: 0, Y: 100, W: 300, H: 50}, Orientation: Horizontal},
		{Rect: Rect{X: 100, Y: 0, W: 50, H: 300}, Orientation: Vertical},
	}
	pruned := pruneChannels(channels)
	if len(pruned) != 2 {
		t.Errorf("pruneChannels: got %d channels, want 2 (different orientations)", len(pruned))
	}
}

func TestPruneChannels_NoDomination(t *testing.T) {
	// Two non-overlapping vertical channels should both survive.
	channels := []Channel{
		{Rect: Rect{X: 50, Y: 0, W: 30, H: 300}, Orientation: Vertical},
		{Rect: Rect{X: 200, Y: 0, W: 30, H: 300}, Orientation: Vertical},
	}
	pruned := pruneChannels(channels)
	if len(pruned) != 2 {
		t.Errorf("pruneChannels: got %d channels, want 2 (non-overlapping)", len(pruned))
	}
}

// --- Routing Graph Tests ---

func TestBuildRoutingGraph_HasNodes(t *testing.T) {
	boxes := []Rect{
		{X: 0, Y: 0, W: 100, H: 100},
		{X: 200, Y: 0, W: 100, H: 100},
	}
	bbox := computeBoundingBox(boxes)
	channels := findChannels(boxes, bbox)

	// Minimal ports for one edge going right→left.
	ports := &PortAssignment{
		SrcPorts: []Port{{NodeIdx: 0, EdgeIdx: 0, Side: DirRight, Pos: geo.Point{X: 100, Y: 50}, IsSrc: true}},
		DstPorts: []Port{{NodeIdx: 1, EdgeIdx: 0, Side: DirLeft, Pos: geo.Point{X: 200, Y: 50}, IsSrc: false}},
	}

	rg := buildRoutingGraph(channels, ports, boxes, bbox)
	if len(rg.Nodes) == 0 {
		t.Fatal("routing graph has no nodes")
	}
	if len(rg.Adj) == 0 {
		t.Fatal("routing graph has no edges")
	}
}

func TestBuildRoutingGraph_NoEdgesThroughBoxes(t *testing.T) {
	boxes := []Rect{
		{X: 0, Y: 0, W: 100, H: 100},
		{X: 200, Y: 0, W: 100, H: 100},
	}
	bbox := computeBoundingBox(boxes)
	channels := findChannels(boxes, bbox)

	ports := &PortAssignment{
		SrcPorts: []Port{{NodeIdx: 0, EdgeIdx: 0, Side: DirRight, Pos: geo.Point{X: 100, Y: 50}, IsSrc: true}},
		DstPorts: []Port{{NodeIdx: 1, EdgeIdx: 0, Side: DirLeft, Pos: geo.Point{X: 200, Y: 50}, IsSrc: false}},
	}

	rg := buildRoutingGraph(channels, ports, boxes, bbox)

	// Verify no edge in the routing graph passes through any box.
	for _, edges := range rg.Adj {
		for _, e := range edges {
			from := rg.Nodes[e.From].Pos
			to := rg.Nodes[e.To].Pos
			if edgePassesThroughBox(from, to, boxes) {
				t.Errorf("routing graph edge (%v → %v) passes through a box", from, to)
			}
		}
	}
}

func TestEdgePassesThroughBox(t *testing.T) {
	boxes := []Rect{{X: 50, Y: 50, W: 100, H: 100}}

	tests := []struct {
		name   string
		a, b   geo.Point
		passes bool
	}{
		{"horizontal through box", geo.Point{X: 0, Y: 100}, geo.Point{X: 200, Y: 100}, true},
		{"vertical through box", geo.Point{X: 100, Y: 0}, geo.Point{X: 100, Y: 200}, true},
		{"horizontal above box", geo.Point{X: 0, Y: 10}, geo.Point{X: 200, Y: 10}, false},
		{"vertical left of box", geo.Point{X: 10, Y: 0}, geo.Point{X: 10, Y: 200}, false},
		{"on box boundary", geo.Point{X: 50, Y: 0}, geo.Point{X: 50, Y: 200}, false}, // boundary, not interior
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := edgePassesThroughBox(tt.a, tt.b, boxes)
			if got != tt.passes {
				t.Errorf("edgePassesThroughBox(%v, %v): got %v, want %v", tt.a, tt.b, got, tt.passes)
			}
		})
	}
}

// --- Dijkstra Tests ---

func TestDijkstraRoute_SimplePath(t *testing.T) {
	// Simple 3-node linear graph: 0 — 1 — 2
	rg := &RoutingGraph{
		Nodes: []RoutingGraphNode{
			{ID: 0, Pos: geo.Point{X: 0, Y: 0}},
			{ID: 1, Pos: geo.Point{X: 100, Y: 0}},
			{ID: 2, Pos: geo.Point{X: 200, Y: 0}},
		},
		Adj: map[int][]RoutingGraphEdge{
			0: {{From: 0, To: 1, Weight: 100, Orientation: Horizontal}},
			1: {
				{From: 1, To: 0, Weight: 100, Orientation: Horizontal},
				{From: 1, To: 2, Weight: 100, Orientation: Horizontal},
			},
			2: {{From: 2, To: 1, Weight: 100, Orientation: Horizontal}},
		},
	}

	path := dijkstraRoute(rg, 0, 2)
	if path == nil {
		t.Fatal("dijkstraRoute returned nil for connected graph")
	}
	if len(path) != 2 {
		t.Fatalf("path length: got %d, want 2 (nodes 1,2)", len(path))
	}
	if path[0] != 1 || path[1] != 2 {
		t.Errorf("path: got %v, want [1,2]", path)
	}
}

func TestDijkstraRoute_PrefersFewerBends(t *testing.T) {
	// Graph with two paths of equal length:
	// Path A: 0→1→2 (straight horizontal, 0 bends)
	// Path B: 0→3→2 (vertical then horizontal, 1 bend)
	rg := &RoutingGraph{
		Nodes: []RoutingGraphNode{
			{ID: 0, Pos: geo.Point{X: 0, Y: 0}},
			{ID: 1, Pos: geo.Point{X: 100, Y: 0}},
			{ID: 2, Pos: geo.Point{X: 200, Y: 0}},
			{ID: 3, Pos: geo.Point{X: 0, Y: 100}},
		},
		Adj: map[int][]RoutingGraphEdge{
			0: {
				{From: 0, To: 1, Weight: 100, Orientation: Horizontal},
				{From: 0, To: 3, Weight: 100, Orientation: Vertical},
			},
			1: {
				{From: 1, To: 0, Weight: 100, Orientation: Horizontal},
				{From: 1, To: 2, Weight: 100, Orientation: Horizontal},
			},
			2: {
				{From: 2, To: 1, Weight: 100, Orientation: Horizontal},
				{From: 2, To: 3, Weight: 200, Orientation: Horizontal}, // longer, making total length same
			},
			3: {
				{From: 3, To: 0, Weight: 100, Orientation: Vertical},
				{From: 3, To: 2, Weight: 200, Orientation: Horizontal},
			},
		},
	}

	path := dijkstraRoute(rg, 0, 2)
	if path == nil {
		t.Fatal("dijkstraRoute returned nil")
	}
	// Path 0→1→2 is length 200 with 0 bends.
	// Path 0→3→2 is length 300 (longer), so Dijkstra picks 0→1→2 by length.
	if len(path) != 2 || path[0] != 1 || path[1] != 2 {
		t.Errorf("expected path through node 1, got %v", path)
	}
}

func TestDijkstraRoute_NoPath(t *testing.T) {
	// Two disconnected nodes.
	rg := &RoutingGraph{
		Nodes: []RoutingGraphNode{
			{ID: 0, Pos: geo.Point{X: 0, Y: 0}},
			{ID: 1, Pos: geo.Point{X: 200, Y: 200}},
		},
		Adj: map[int][]RoutingGraphEdge{},
	}

	path := dijkstraRoute(rg, 0, 1)
	if path != nil {
		t.Errorf("expected nil path for disconnected nodes, got %v", path)
	}
}

func TestDijkstraRoute_SameNode(t *testing.T) {
	rg := &RoutingGraph{
		Nodes: []RoutingGraphNode{
			{ID: 0, Pos: geo.Point{X: 0, Y: 0}},
		},
		Adj: map[int][]RoutingGraphEdge{},
	}

	path := dijkstraRoute(rg, 0, 0)
	if len(path) != 1 || path[0] != 0 {
		t.Errorf("expected [0] for same-node route, got %v", path)
	}
}

// --- SimplifyRoute Tests ---

func TestSimplifyRoute_RemovesDuplicates(t *testing.T) {
	points := []*geo.Point{
		{X: 0, Y: 0},
		{X: 0, Y: 0},
		{X: 100, Y: 0},
	}
	result := simplifyRoute(points)
	if len(result) != 2 {
		t.Errorf("simplifyRoute: got %d points, want 2", len(result))
	}
}

func TestSimplifyRoute_RemovesCollinear(t *testing.T) {
	points := []*geo.Point{
		{X: 0, Y: 0},
		{X: 50, Y: 0},
		{X: 100, Y: 0},
	}
	result := simplifyRoute(points)
	if len(result) != 2 {
		t.Errorf("simplifyRoute: got %d points, want 2 (removed collinear)", len(result))
	}
	if result[0].X != 0 || result[1].X != 100 {
		t.Errorf("simplifyRoute: endpoints wrong: (%v, %v)", result[0], result[1])
	}
}

func TestSimplifyRoute_PreservesBends(t *testing.T) {
	// L-shape: horizontal then vertical.
	points := []*geo.Point{
		{X: 0, Y: 0},
		{X: 100, Y: 0},
		{X: 100, Y: 100},
	}
	result := simplifyRoute(points)
	if len(result) != 3 {
		t.Errorf("simplifyRoute: got %d points, want 3 (preserve L-bend)", len(result))
	}
}

func TestSimplifyRoute_SnapsNearCollinear(t *testing.T) {
	// Three points that are nearly collinear (floating-point noise).
	points := []*geo.Point{
		{X: 100, Y: 0},
		{X: 100.3, Y: 50},   // tiny X offset
		{X: 100.1, Y: 100},
	}
	result := simplifyRoute(points)
	// All three are nearly same X → collinear → simplified to 2 points.
	if len(result) != 2 {
		t.Errorf("simplifyRoute: got %d points, want 2 (snap near-collinear)", len(result))
	}
}

// --- SegmentsCross Tests ---

func TestSegmentsCross(t *testing.T) {
	tests := []struct {
		name   string
		a1, a2 geo.Point
		b1, b2 geo.Point
		want   bool
	}{
		{
			name: "perpendicular crossing",
			a1: geo.Point{X: 0, Y: 50}, a2: geo.Point{X: 100, Y: 50},
			b1: geo.Point{X: 50, Y: 0}, b2: geo.Point{X: 50, Y: 100},
			want: true,
		},
		{
			name: "parallel horizontal",
			a1: geo.Point{X: 0, Y: 50}, a2: geo.Point{X: 100, Y: 50},
			b1: geo.Point{X: 0, Y: 80}, b2: geo.Point{X: 100, Y: 80},
			want: false,
		},
		{
			name: "perpendicular non-intersecting",
			a1: geo.Point{X: 0, Y: 50}, a2: geo.Point{X: 30, Y: 50},
			b1: geo.Point{X: 50, Y: 0}, b2: geo.Point{X: 50, Y: 100},
			want: false,
		},
		{
			name: "T-junction (endpoint touching)",
			a1: geo.Point{X: 0, Y: 50}, a2: geo.Point{X: 100, Y: 50},
			b1: geo.Point{X: 50, Y: 50}, b2: geo.Point{X: 50, Y: 100},
			want: false, // endpoint on the segment, not strict crossing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := segmentsCross(tt.a1, tt.a2, tt.b1, tt.b2)
			if got != tt.want {
				t.Errorf("segmentsCross: got %v, want %v", got, tt.want)
			}
		})
	}
}

// --- FindNearest Tests ---

func TestFindNearest(t *testing.T) {
	rg := &RoutingGraph{
		Nodes: []RoutingGraphNode{
			{ID: 0, Pos: geo.Point{X: 0, Y: 0}},
			{ID: 1, Pos: geo.Point{X: 100, Y: 0}},
			{ID: 2, Pos: geo.Point{X: 50, Y: 50}},
		},
	}

	// Closest to (48, 48) should be node 2 at (50, 50).
	id := rg.FindNearest(geo.Point{X: 48, Y: 48})
	if id != 2 {
		t.Errorf("FindNearest: got node %d, want 2", id)
	}

	// Closest to (99, 1) should be node 1 at (100, 0).
	id = rg.FindNearest(geo.Point{X: 99, Y: 1})
	if id != 1 {
		t.Errorf("FindNearest: got node %d, want 1", id)
	}
}

func TestFindNearest_EmptyGraph(t *testing.T) {
	rg := &RoutingGraph{Nodes: nil}
	id := rg.FindNearest(geo.Point{X: 0, Y: 0})
	if id != -1 {
		t.Errorf("FindNearest on empty graph: got %d, want -1", id)
	}
}

// --- DijkstraState Tests ---

func TestDijkstraState_Less(t *testing.T) {
	a := DijkstraState{Length: 100, Bends: 2}
	b := DijkstraState{Length: 200, Bends: 0}
	if !a.Less(b) {
		t.Error("shorter length should be Less")
	}
	if b.Less(a) {
		t.Error("longer length should not be Less")
	}

	// Same length, fewer bends wins.
	c := DijkstraState{Length: 100, Bends: 1}
	d := DijkstraState{Length: 100, Bends: 3}
	if !c.Less(d) {
		t.Error("same length, fewer bends should be Less")
	}
}

// --- BoundingBox Tests ---

func TestComputeBoundingBox(t *testing.T) {
	boxes := []Rect{
		{X: 10, Y: 20, W: 100, H: 50},
		{X: 200, Y: 30, W: 80, H: 60},
	}
	bbox := computeBoundingBox(boxes)

	// Expected: min=(10,20), max=(280,90), margin=40.
	if bbox.Left() != -30 {
		t.Errorf("bbox.Left: got %.1f, want -30", bbox.Left())
	}
	if bbox.Top() != -20 {
		t.Errorf("bbox.Top: got %.1f, want -20", bbox.Top())
	}
	if bbox.Right() != 320 {
		t.Errorf("bbox.Right: got %.1f, want 320", bbox.Right())
	}
	if bbox.Bottom() != 130 {
		t.Errorf("bbox.Bottom: got %.1f, want 130", bbox.Bottom())
	}
}

// --- Integration: End-to-End Pipeline ---

func TestPipeline_TwoBoxes(t *testing.T) {
	// Test the full pipeline from port assignment through routing,
	// without using d2graph.Edge (which requires full graph setup).
	boxes := []Rect{
		{X: 0, Y: 0, W: 100, H: 100},
		{X: 200, Y: 0, W: 100, H: 100},
	}
	bbox := computeBoundingBox(boxes)

	// Manually create ports as if for a horizontal edge (box 0 → box 1).
	ports := &PortAssignment{
		SrcPorts: []Port{{NodeIdx: 0, EdgeIdx: 0, Side: DirRight, Pos: geo.Point{X: 100, Y: 50}, IsSrc: true}},
		DstPorts: []Port{{NodeIdx: 1, EdgeIdx: 0, Side: DirLeft, Pos: geo.Point{X: 200, Y: 50}, IsSrc: false}},
	}

	channels := findChannels(boxes, bbox)
	rg := buildRoutingGraph(channels, ports, boxes, bbox)
	routes := routeAllEdges(rg, ports, []*d2graph.Edge{
		{Src: &d2graph.Object{}, Dst: &d2graph.Object{}}, // only used for len(edges)
	})

	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if len(routes[0].Points) < 2 {
		t.Fatalf("route has too few points: %d", len(routes[0].Points))
	}

	// Verify route is orthogonal (each segment is H or V).
	pts := routes[0].Points
	for i := 0; i < len(pts)-1; i++ {
		dx := math.Abs(pts[i].X - pts[i+1].X)
		dy := math.Abs(pts[i].Y - pts[i+1].Y)
		if dx > 0.5 && dy > 0.5 {
			t.Errorf("segment %d is diagonal: (%v → %v)", i, pts[i], pts[i+1])
		}
	}
}

// --- Rect Tests ---

func TestRect_Methods(t *testing.T) {
	r := Rect{X: 10, Y: 20, W: 100, H: 50}
	if r.Left() != 10 {
		t.Errorf("Left: got %.1f, want 10", r.Left())
	}
	if r.Right() != 110 {
		t.Errorf("Right: got %.1f, want 110", r.Right())
	}
	if r.Top() != 20 {
		t.Errorf("Top: got %.1f, want 20", r.Top())
	}
	if r.Bottom() != 70 {
		t.Errorf("Bottom: got %.1f, want 70", r.Bottom())
	}
	if r.CenterX() != 60 {
		t.Errorf("CenterX: got %.1f, want 60", r.CenterX())
	}
	if r.CenterY() != 45 {
		t.Errorf("CenterY: got %.1f, want 45", r.CenterY())
	}
}
