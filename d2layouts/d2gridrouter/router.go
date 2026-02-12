// [FORK] Orthogonal grid edge router.
// Implements stages 2-5 of the Hegemann & Wolff (2023) pipeline for
// routing edges orthogonally when node positions are fixed (e.g., grid layout).
//
// Pipeline:
//   Stage 2: Port assignment — determine where edges exit/enter cells
//   Stage 3: Channel construction + routing graph + modified Dijkstra
//   Stage 4: Path ordering on shared segments
//   Stage 5: Constrained nudging (balance inter-edge distances)
//
// Reference: arXiv:2309.01671, github.com/WueGD/wueortho

package d2gridrouter

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/log"
)

// RouteEdges routes edges orthogonally through grid corridors.
// Nodes must already be positioned. Only edge routes are computed.
func RouteEdges(ctx context.Context, g *d2graph.Graph, edges []*d2graph.Edge) error {
	if len(edges) == 0 {
		return nil
	}

	log.Debug(ctx, "grid router: routing edges", slog.Any("count", len(edges)))

	// Collect all grid siblings as obstacle boxes (not just edge endpoints).
	parent := findCommonParent(edges)
	var allObjects []*d2graph.Object
	if parent != nil && len(parent.ChildrenArray) > 0 {
		allObjects = parent.ChildrenArray
	} else {
		seen := make(map[*d2graph.Object]bool)
		for _, e := range edges {
			if !seen[e.Src] {
				seen[e.Src] = true
				allObjects = append(allObjects, e.Src)
			}
			if !seen[e.Dst] {
				seen[e.Dst] = true
				allObjects = append(allObjects, e.Dst)
			}
		}
	}

	if len(allObjects) == 0 {
		return fmt.Errorf("grid router: no objects found")
	}

	// Build node index and boxes.
	nodeIndex := make(map[*d2graph.Object]int)
	boxes := make([]Rect, len(allObjects))
	for i, obj := range allObjects {
		nodeIndex[obj] = i
		boxes[i] = Rect{
			X: obj.TopLeft.X,
			Y: obj.TopLeft.Y,
			W: obj.Width,
			H: obj.Height,
		}
	}

	// Compute bounding box with margin for boundary corridors.
	bbox := computeBoundingBox(boxes)

	// Stage 2: Port assignment.
	ports := assignPorts(boxes, edges, nodeIndex)

	// Stage 3a: Channel construction.
	channels := findChannels(boxes, bbox)

	// Stage 3b: Build routing graph (partial grid).
	rg := buildRoutingGraph(channels, ports, boxes, bbox)

	// Stage 3c: Route each edge via modified Dijkstra.
	routes := routeAllEdges(rg, ports, edges)

	// Stages 4+5: Nudge parallel edges apart in shared corridors.
	// Edges sharing a corridor are evenly distributed across its width.
	nudgeRoutes(routes, channels, boxes)

	// Apply routes to edges.
	// Port positions are already on shape boundaries, so we use them directly
	// without TraceToShape (which would clip to a different boundary point
	// and create visible kinks).
	for _, route := range routes {
		edge := edges[route.EdgeIdx]
		if len(route.Points) >= 2 {
			edge.Route = simplifyRoute(route.Points)
		} else {
			// Fallback to straight line.
			edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
			edge.TraceToShape(edge.Route, 0, 1)
		}
		if edge.Label.Value != "" {
			edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}

	return nil
}

// findCommonParent returns the common parent of all edge endpoints.
func findCommonParent(edges []*d2graph.Edge) *d2graph.Object {
	if len(edges) == 0 {
		return nil
	}
	parent := edges[0].Src.Parent
	for _, e := range edges {
		if e.Src.Parent != parent || e.Dst.Parent != parent {
			return nil
		}
	}
	return parent
}

// computeBoundingBox computes bounding box of all node boxes with margin.
func computeBoundingBox(boxes []Rect) Rect {
	if len(boxes) == 0 {
		return Rect{}
	}
	minX, minY := boxes[0].X, boxes[0].Y
	maxX, maxY := boxes[0].Right(), boxes[0].Bottom()
	for _, b := range boxes[1:] {
		if b.X < minX {
			minX = b.X
		}
		if b.Y < minY {
			minY = b.Y
		}
		if b.Right() > maxX {
			maxX = b.Right()
		}
		if b.Bottom() > maxY {
			maxY = b.Bottom()
		}
	}
	// Add margin for boundary corridors.
	margin := 40.0
	return Rect{
		X: minX - margin,
		Y: minY - margin,
		W: (maxX - minX) + 2*margin,
		H: (maxY - minY) + 2*margin,
	}
}

// routeAllEdges routes all edges through the routing graph.
// Edges are routed in priority order: same-column/row (simple) edges first,
// then longer cross-grid edges. After each edge, crossing penalties are
// added to routing graph edges that would cause crossings with already-routed
// edges. This encourages subsequent edges to choose alternative corridors.
func routeAllEdges(rg *RoutingGraph, ports *PortAssignment, edges []*d2graph.Edge) []EdgeRoute {
	routes := make([]EdgeRoute, len(edges))

	// Determine routing order: prioritize direct edges, then by Manhattan distance.
	order := edgeRoutingOrder(ports, edges)

	// Track segments of already-routed edges for crossing detection.
	var routedSegments []routedSeg

	for _, i := range order {
		srcPort := ports.SrcPorts[i]
		dstPort := ports.DstPorts[i]

		srcNodeID := rg.FindNearest(srcPort.Pos)
		dstNodeID := rg.FindNearest(dstPort.Pos)

		if srcNodeID < 0 || dstNodeID < 0 || srcNodeID == dstNodeID {
			routes[i] = EdgeRoute{
				EdgeIdx: i,
				Points:  []*geo.Point{{X: srcPort.Pos.X, Y: srcPort.Pos.Y}, {X: dstPort.Pos.X, Y: dstPort.Pos.Y}},
			}
			continue
		}

		// Add crossing penalties before routing this edge.
		addCrossingPenalties(rg, routedSegments)

		path := dijkstraRoute(rg, srcNodeID, dstNodeID)

		// Remove penalties after routing.
		removeCrossingPenalties(rg, routedSegments)

		if len(path) == 0 {
			routes[i] = EdgeRoute{
				EdgeIdx: i,
				Points:  []*geo.Point{{X: srcPort.Pos.X, Y: srcPort.Pos.Y}, {X: dstPort.Pos.X, Y: dstPort.Pos.Y}},
			}
			continue
		}

		// Convert node path to points.
		points := make([]*geo.Point, 0, len(path)+2)
		points = append(points, &geo.Point{X: srcPort.Pos.X, Y: srcPort.Pos.Y})
		for _, nodeID := range path {
			n := rg.Nodes[nodeID]
			points = append(points, &geo.Point{X: n.Pos.X, Y: n.Pos.Y})
		}
		points = append(points, &geo.Point{X: dstPort.Pos.X, Y: dstPort.Pos.Y})
		points = simplifyRoute(points)

		routes[i] = EdgeRoute{EdgeIdx: i, Points: points}

		// Record this edge's segments for future crossing detection.
		for j := 0; j < len(path)-1; j++ {
			routedSegments = append(routedSegments, routedSeg{from: path[j], to: path[j+1]})
		}
	}
	return routes
}

// routedSeg represents a segment of an already-routed edge in the routing graph.
type routedSeg struct {
	from, to int // routing graph node IDs
}

// crossingPenalty is added to routing graph edges that would cross already-routed edges.
const crossingPenalty = 500.0

// addCrossingPenalties increases weights on routing graph edges that cross
// already-routed segments (perpendicular intersections).
func addCrossingPenalties(rg *RoutingGraph, routed []routedSeg) {
	if len(routed) == 0 {
		return
	}
	for nodeID, edges := range rg.Adj {
		for ei := range edges {
			edge := &rg.Adj[nodeID][ei]
			// Process each undirected edge only once (from the lower nodeID side).
			if edge.From != nodeID {
				continue
			}
			fromPos := rg.Nodes[edge.From].Pos
			toPos := rg.Nodes[edge.To].Pos

			for _, rs := range routed {
				rsFrom := rg.Nodes[rs.from].Pos
				rsTo := rg.Nodes[rs.to].Pos
				if segmentsCross(fromPos, toPos, rsFrom, rsTo) {
					edge.Weight += crossingPenalty
					// Also apply to the reverse direction in adj list.
					for ri := range rg.Adj[edge.To] {
						rev := &rg.Adj[edge.To][ri]
						if rev.From == edge.To && rev.To == edge.From {
							rev.Weight += crossingPenalty
							break
						}
					}
					break
				}
			}
		}
	}
}

// removeCrossingPenalties reverses the penalties added by addCrossingPenalties.
func removeCrossingPenalties(rg *RoutingGraph, routed []routedSeg) {
	if len(routed) == 0 {
		return
	}
	for nodeID, edges := range rg.Adj {
		for ei := range edges {
			edge := &rg.Adj[nodeID][ei]
			if edge.From != nodeID {
				continue
			}
			fromPos := rg.Nodes[edge.From].Pos
			toPos := rg.Nodes[edge.To].Pos

			for _, rs := range routed {
				rsFrom := rg.Nodes[rs.from].Pos
				rsTo := rg.Nodes[rs.to].Pos
				if segmentsCross(fromPos, toPos, rsFrom, rsTo) {
					edge.Weight -= crossingPenalty
					for ri := range rg.Adj[edge.To] {
						rev := &rg.Adj[edge.To][ri]
						if rev.From == edge.To && rev.To == edge.From {
							rev.Weight -= crossingPenalty
							break
						}
					}
					break
				}
			}
		}
	}
}

// segmentsCross returns true if two orthogonal segments cross each other
// (one horizontal, one vertical, and they intersect).
func segmentsCross(a1, a2, b1, b2 geo.Point) bool {
	const eps = 0.5
	aHoriz := math.Abs(a1.Y-a2.Y) < eps
	bHoriz := math.Abs(b1.Y-b2.Y) < eps

	if aHoriz == bHoriz {
		return false // parallel segments don't cross
	}

	// One is horizontal, one is vertical.
	var hStart, hEnd, vStart, vEnd geo.Point
	if aHoriz {
		hStart, hEnd = a1, a2
		vStart, vEnd = b1, b2
	} else {
		hStart, hEnd = b1, b2
		vStart, vEnd = a1, a2
	}

	// Normalize ranges.
	hMinX := math.Min(hStart.X, hEnd.X)
	hMaxX := math.Max(hStart.X, hEnd.X)
	vMinY := math.Min(vStart.Y, vEnd.Y)
	vMaxY := math.Max(vStart.Y, vEnd.Y)

	// Check if vertical segment's X is within horizontal range,
	// and horizontal segment's Y is within vertical range.
	return vStart.X > hMinX && vStart.X < hMaxX &&
		hStart.Y > vMinY && hStart.Y < vMaxY
}

// simplifyRoute removes duplicate and intermediate collinear points.
// Uses a small tolerance for floating-point coordinates from the routing graph.
func simplifyRoute(points []*geo.Point) []*geo.Point {
	if len(points) <= 1 {
		return points
	}
	const tol = 0.5 // tolerance for "same coordinate" comparison

	nearEq := func(a, b float64) bool {
		return math.Abs(a-b) < tol
	}

	// First pass: remove consecutive duplicates.
	deduped := []*geo.Point{points[0]}
	for i := 1; i < len(points); i++ {
		prev := deduped[len(deduped)-1]
		if !nearEq(points[i].X, prev.X) || !nearEq(points[i].Y, prev.Y) {
			deduped = append(deduped, points[i])
		}
	}
	if len(deduped) <= 2 {
		return deduped
	}
	// Second pass: remove collinear intermediate points.
	// Also snap near-collinear points to exact alignment.
	result := []*geo.Point{deduped[0]}
	for i := 1; i < len(deduped)-1; i++ {
		prev := result[len(result)-1]
		next := deduped[i+1]
		curr := deduped[i]
		sameX := nearEq(prev.X, curr.X) && nearEq(curr.X, next.X)
		sameY := nearEq(prev.Y, curr.Y) && nearEq(curr.Y, next.Y)
		if sameX || sameY {
			// Collinear — skip this intermediate point.
			continue
		}
		// Not collinear — this is a real bend. Snap to alignment with neighbors.
		if nearEq(prev.X, curr.X) {
			curr.X = prev.X // snap vertical segment
		} else if nearEq(prev.Y, curr.Y) {
			curr.Y = prev.Y // snap horizontal segment
		}
		if nearEq(curr.X, next.X) {
			curr.X = next.X
		} else if nearEq(curr.Y, next.Y) {
			curr.Y = next.Y
		}
		result = append(result, curr)
	}
	// Snap last point to alignment with previous.
	last := deduped[len(deduped)-1]
	if len(result) > 0 {
		prevPt := result[len(result)-1]
		if nearEq(prevPt.X, last.X) {
			last.X = prevPt.X
		} else if nearEq(prevPt.Y, last.Y) {
			last.Y = prevPt.Y
		}
	}
	result = append(result, last)
	return result
}

// FindNearest returns the ID of the routing graph node closest to the given point.
// Returns -1 if the routing graph has no nodes.
func (rg *RoutingGraph) FindNearest(p geo.Point) int {
	if len(rg.Nodes) == 0 {
		return -1
	}
	bestID := 0
	bestDist := distSq(rg.Nodes[0].Pos, p)
	for i := 1; i < len(rg.Nodes); i++ {
		d := distSq(rg.Nodes[i].Pos, p)
		if d < bestDist {
			bestDist = d
			bestID = i
		}
	}
	return bestID
}

func distSq(a, b geo.Point) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return dx*dx + dy*dy
}

// edgeRoutingOrder returns indices of edges sorted for optimal routing order.
// Same-column (vertical) and same-row (horizontal) edges are routed first
// since they have simple, direct paths. Cross-grid edges are sorted by
// Manhattan distance (longest first) so they get first pick of corridors.
func edgeRoutingOrder(ports *PortAssignment, edges []*d2graph.Edge) []int {
	type edgePriority struct {
		idx      int
		priority int     // 0 = same col/row, 1 = cross-grid
		dist     float64 // Manhattan distance (negated for descending sort)
	}

	priorities := make([]edgePriority, len(edges))
	for i := range edges {
		src := ports.SrcPorts[i]
		dst := ports.DstPorts[i]
		dx := math.Abs(src.Pos.X - dst.Pos.X)
		dy := math.Abs(src.Pos.Y - dst.Pos.Y)

		// Same side pair = direct edge (src exits bottom, dst enters top of same column, etc.)
		sameAxis := (src.Side == DirBottom && dst.Side == DirTop) ||
			(src.Side == DirTop && dst.Side == DirBottom) ||
			(src.Side == DirRight && dst.Side == DirLeft) ||
			(src.Side == DirLeft && dst.Side == DirRight)

		p := 1
		if sameAxis {
			p = 0
		}

		priorities[i] = edgePriority{
			idx:      i,
			priority: p,
			dist:     dx + dy,
		}
	}

	sort.Slice(priorities, func(a, b int) bool {
		if priorities[a].priority != priorities[b].priority {
			return priorities[a].priority < priorities[b].priority
		}
		// Within same priority: longer edges first (they need better corridors).
		return priorities[a].dist > priorities[b].dist
	})

	order := make([]int, len(edges))
	for i, p := range priorities {
		order[i] = p.idx
	}
	return order
}

// sortedUniqueFloats returns sorted unique values from a slice.
func sortedUniqueFloats(vals []float64) []float64 {
	if len(vals) == 0 {
		return nil
	}
	sort.Float64s(vals)
	result := []float64{vals[0]}
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1] {
			result = append(result, vals[i])
		}
	}
	return result
}
