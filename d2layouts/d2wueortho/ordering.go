// [FORK] Stage 4: Path Ordering (Hegemann & Wolff §3.4).
// Orders edges that share routing graph segments to minimize crossings.
//
// When multiple edges traverse the same corridor (routing graph edge),
// they need a consistent ordering so that the subsequent nudging stage
// can separate them properly without introducing additional crossings.
//
// Algorithm:
// 1. Identify shared segments: routing graph edges used by >1 routed edge.
// 2. Assign default ordering direction: LEFT for horizontal bundles, DOWN for vertical.
// 3. Order edges within each bundle by their source/destination positions
//    relative to the ordering direction.
// 4. Return ordering keys so nudging distributes edges in the computed order.

package d2wueortho

import (
	"math"
	"sort"

	"oss.terrastruct.com/d2/lib/geo"
)

// EdgeOrdering stores perpendicular sort keys for edges on shared segments.
// Keys are (edgeIdx, orientation) → perpendicular position.
// Nudging uses these to sort edges within bundles in the correct order,
// avoiding mid-segment crossings.
type EdgeOrdering struct {
	// hKeys[edgeIdx] = sort key for horizontal shared segments (Y position).
	// vKeys[edgeIdx] = sort key for vertical shared segments (X position).
	hKeys map[int]float64
	vKeys map[int]float64
}

// HKey returns the horizontal sort key for an edge (perpendicular Y position).
// Returns +Inf if no ordering was computed (edge not in any shared horizontal segment).
func (eo *EdgeOrdering) HKey(edgeIdx int) float64 {
	if eo == nil || eo.hKeys == nil {
		return math.MaxFloat64
	}
	if k, ok := eo.hKeys[edgeIdx]; ok {
		return k
	}
	return math.MaxFloat64
}

// VKey returns the vertical sort key for an edge (perpendicular X position).
func (eo *EdgeOrdering) VKey(edgeIdx int) float64 {
	if eo == nil || eo.vKeys == nil {
		return math.MaxFloat64
	}
	if k, ok := eo.vKeys[edgeIdx]; ok {
		return k
	}
	return math.MaxFloat64
}

// sharedSegment represents a routing graph edge used by multiple routed edges.
type sharedSegment struct {
	from, to    int         // routing graph node IDs
	orientation Orientation // H or V
	edgeIndices []int       // which routed edges use this segment
}

// orderEdgesOnSharedSegments computes ordering keys for edges within shared segments
// so that nudging distributes them without introducing mid-segment crossings.
//
// For horizontal shared segments: edges are sorted by their Y at the nearest endpoint.
// For vertical shared segments: edges are sorted by their X at the nearest endpoint.
//
// Returns an EdgeOrdering that nudging uses to sort edges within bundles.
func orderEdgesOnSharedSegments(routes []EdgeRoute, rg *RoutingGraph) *EdgeOrdering {
	ordering := &EdgeOrdering{
		hKeys: make(map[int]float64),
		vKeys: make(map[int]float64),
	}

	if len(routes) <= 1 || rg == nil {
		return ordering
	}

	// Step 1: Build a map of routing graph edges → which routed edges use them.
	type rgEdgeKey struct {
		from, to int // canonical: from < to
	}
	segmentUsers := make(map[rgEdgeKey][]int)

	for ri, route := range routes {
		pts := route.Points
		if len(pts) < 2 {
			continue
		}
		// Walk through consecutive point pairs and find nearest routing graph nodes.
		for i := 0; i < len(pts)-1; i++ {
			fromID := rg.FindNearest(*pts[i])
			toID := rg.FindNearest(*pts[i+1])
			if fromID < 0 || toID < 0 || fromID == toID {
				continue
			}
			key := rgEdgeKey{fromID, toID}
			if key.from > key.to {
				key.from, key.to = key.to, key.from
			}
			segmentUsers[key] = append(segmentUsers[key], ri)
		}
	}

	// Step 2: For each shared segment, determine direction and sort edges.
	for key, users := range segmentUsers {
		if len(users) <= 1 {
			continue
		}

		// Deduplicate users.
		seen := make(map[int]bool)
		deduped := users[:0]
		for _, u := range users {
			if !seen[u] {
				seen[u] = true
				deduped = append(deduped, u)
			}
		}
		users = deduped
		if len(users) <= 1 {
			continue
		}

		fromPos := rg.Nodes[key.from].Pos
		toPos := rg.Nodes[key.to].Pos

		// Determine orientation.
		isHorizontal := absF(fromPos.Y-toPos.Y) < absF(fromPos.X-toPos.X)

		// Sort edges by perpendicular position.
		sort.Slice(users, func(i, j int) bool {
			ri, rj := routes[users[i]], routes[users[j]]
			if isHorizontal {
				return edgeSortKey(ri, true) < edgeSortKey(rj, true)
			}
			return edgeSortKey(ri, false) < edgeSortKey(rj, false)
		})

		// Step 3: Store ordering keys — the sorted position index becomes the key.
		// This ensures nudging distributes edges in the computed order.
		for rank, edgeIdx := range users {
			if isHorizontal {
				// Only store if this is the first (or better) ordering for this edge.
				if _, exists := ordering.hKeys[edgeIdx]; !exists {
					ordering.hKeys[edgeIdx] = float64(rank)
				}
			} else {
				if _, exists := ordering.vKeys[edgeIdx]; !exists {
					ordering.vKeys[edgeIdx] = float64(rank)
				}
			}
		}
	}

	return ordering
}

// edgeSortKey returns the position used for sorting an edge within a shared segment.
// If useY is true, returns the Y coordinate of the edge's first point (for horizontal segments).
// If useY is false, returns the X coordinate (for vertical segments).
func edgeSortKey(route EdgeRoute, useY bool) float64 {
	if len(route.Points) == 0 {
		return 0
	}
	if useY {
		return route.Points[0].Y
	}
	return route.Points[0].X
}

// absF returns the absolute value of a float64.
func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// FindNearestPoint returns the ID of the routing graph node closest to a geo.Point.
func (rg *RoutingGraph) FindNearestPoint(p geo.Point) int {
	return rg.FindNearest(p)
}
