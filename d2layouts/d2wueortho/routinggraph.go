// [FORK] Stage 3b: Routing Graph Construction (Hegemann & Wolff §3.3).
// Builds a partial grid from channel representatives.
//
// The routing graph is a planar graph where:
// - Vertices are at ports and at intersection points of H/V representatives.
// - Edges connect consecutive vertices along the same representative.
// - There are no vertices or edges inside node boxes (gaps).
//
// This ensures that any path through the routing graph produces an
// orthogonal route that avoids all node boxes.

package d2wueortho

import (
	"math"
	"sort"

	"oss.terrastruct.com/d2/lib/geo"
)

const epsilon = 0.5 // tolerance for coordinate comparison

// buildRoutingGraph constructs the partial grid from channels and ports.
func buildRoutingGraph(channels []Channel, ports *PortAssignment, boxes []Rect, bbox Rect) *RoutingGraph {
	// Build representatives from channels.
	segments := buildRepresentatives(channels, ports)
	sortSegmentPoints(segments)
	segments = deduplicateSegments(segments)

	// Separate into horizontal and vertical segments.
	var hSegs, vSegs []Segment
	for _, s := range segments {
		if s.Orientation == Horizontal {
			hSegs = append(hSegs, s)
		} else {
			vSegs = append(vSegs, s)
		}
	}

	// Find all intersection points between H and V segments.
	type pointKey struct{ x, y float64 }
	nodeMap := make(map[pointKey]int)
	var nodes []RoutingGraphNode

	addNode := func(p geo.Point) int {
		// Snap to grid to avoid floating point issues.
		px := math.Round(p.X*100) / 100
		py := math.Round(p.Y*100) / 100
		key := pointKey{px, py}
		if id, ok := nodeMap[key]; ok {
			return id
		}
		id := len(nodes)
		nodes = append(nodes, RoutingGraphNode{ID: id, Pos: geo.Point{X: px, Y: py}})
		nodeMap[key] = id
		return id
	}

	// Add port positions as nodes.
	for _, p := range ports.SrcPorts {
		addNode(p.Pos)
	}
	for _, p := range ports.DstPorts {
		addNode(p.Pos)
	}

	// For each pair of (H segment, V segment), check intersection.
	for _, h := range hSegs {
		for _, v := range vSegs {
			// H segment: from (h.Start.X, h.Start.Y) to (h.End.X, h.Start.Y) [same Y]
			// V segment: from (v.Start.X, v.Start.Y) to (v.Start.X, v.End.Y) [same X]
			hY := h.Start.Y
			vX := v.Start.X

			if vX >= h.Start.X-epsilon && vX <= h.End.X+epsilon &&
				hY >= v.Start.Y-epsilon && hY <= v.End.Y+epsilon {
				addNode(geo.Point{X: vX, Y: hY})
			}
		}
	}

	// Also add segment endpoints.
	for _, s := range segments {
		addNode(s.Start)
		addNode(s.End)
	}

	// Build edges: connect consecutive nodes along each segment.
	adj := make(map[int][]RoutingGraphEdge)

	connectAlongSegment := func(seg Segment) {
		// Find all nodes that lie on this segment.
		var onSeg []int
		for id, n := range nodes {
			if seg.Orientation == Horizontal {
				if math.Abs(n.Pos.Y-seg.Start.Y) < epsilon &&
					n.Pos.X >= seg.Start.X-epsilon &&
					n.Pos.X <= seg.End.X+epsilon {
					onSeg = append(onSeg, id)
				}
			} else {
				if math.Abs(n.Pos.X-seg.Start.X) < epsilon &&
					n.Pos.Y >= seg.Start.Y-epsilon &&
					n.Pos.Y <= seg.End.Y+epsilon {
					onSeg = append(onSeg, id)
				}
			}
		}

		// Sort by position along segment.
		if seg.Orientation == Horizontal {
			sort.Slice(onSeg, func(i, j int) bool {
				return nodes[onSeg[i]].Pos.X < nodes[onSeg[j]].Pos.X
			})
		} else {
			sort.Slice(onSeg, func(i, j int) bool {
				return nodes[onSeg[i]].Pos.Y < nodes[onSeg[j]].Pos.Y
			})
		}

		// Connect consecutive nodes.
		for k := 0; k < len(onSeg)-1; k++ {
			a := onSeg[k]
			b := onSeg[k+1]
			w := math.Hypot(nodes[b].Pos.X-nodes[a].Pos.X, nodes[b].Pos.Y-nodes[a].Pos.Y)
			if w < epsilon {
				continue
			}

			// Check that this edge doesn't pass through any box.
			if edgePassesThroughBox(nodes[a].Pos, nodes[b].Pos, boxes) {
				continue
			}

			edge := RoutingGraphEdge{From: a, To: b, Weight: w, Orientation: seg.Orientation}
			adj[a] = append(adj[a], edge)
			// Add reverse edge (undirected graph).
			revEdge := RoutingGraphEdge{From: b, To: a, Weight: w, Orientation: seg.Orientation}
			adj[b] = append(adj[b], revEdge)
		}
	}

	for _, s := range segments {
		connectAlongSegment(s)
	}

	return &RoutingGraph{Nodes: nodes, Adj: adj}
}

// edgePassesThroughBox checks if the segment from a to b passes through
// the interior of any box. Both a and b are on representative lines,
// so the segment is axis-aligned.
func edgePassesThroughBox(a, b geo.Point, boxes []Rect) bool {
	for _, box := range boxes {
		if math.Abs(a.X-b.X) < epsilon {
			// Vertical segment.
			x := (a.X + b.X) / 2
			minY := math.Min(a.Y, b.Y)
			maxY := math.Max(a.Y, b.Y)
			// Check if vertical line at x passes through box interior.
			if x > box.Left()+epsilon && x < box.Right()-epsilon {
				if maxY > box.Top()+epsilon && minY < box.Bottom()-epsilon {
					return true
				}
			}
		} else if math.Abs(a.Y-b.Y) < epsilon {
			// Horizontal segment.
			y := (a.Y + b.Y) / 2
			minX := math.Min(a.X, b.X)
			maxX := math.Max(a.X, b.X)
			if y > box.Top()+epsilon && y < box.Bottom()-epsilon {
				if maxX > box.Left()+epsilon && minX < box.Right()-epsilon {
					return true
				}
			}
		}
	}
	return false
}
