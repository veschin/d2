// [FORK] Stage 5: Constraint-based Nudging (Hegemann & Wolff §3.5).
//
// Separates edges that share corridor segments by building a constraint DAG
// and solving via topological sort + longest path. This produces better results
// than simple even distribution by respecting minimum-distance constraints
// between adjacent segments.
//
// Algorithm:
// 1. Decompose routes into H/V segments.
// 2. Group overlapping segments into bundles (shared corridors).
// 3. For each bundle: build constraint DAG ordering segments by position.
// 4. Add minimum-distance arcs (EdgeSpacing) between adjacent segments.
// 5. Solve via topological sort + longest-path to get optimal positions.
// 6. Fallback to even distribution if DAG solving fails.

package d2wueortho

import (
	"math"
	"sort"
)

// edgeSegment represents one horizontal or vertical segment of an edge route.
type edgeSegment struct {
	edgeIdx     int         // which edge this belongs to
	segIdx      int         // index within the edge's segment list
	orientation Orientation // H or V
	fixedCoord  float64     // the shared coordinate (Y for H, X for V)
	rangeMin    float64     // start of the varying coordinate
	rangeMax    float64     // end of the varying coordinate
}

// segmentBundle groups segments that share a corridor.
type segmentBundle struct {
	orientation Orientation
	fixedCoord  float64 // the coordinate all segments share
	segments    []edgeSegment
}

// constraintArc represents a minimum-distance constraint: node A must be at
// least `minDist` before node B in the assigned coordinate.
type constraintArc struct {
	from, to int
	minDist  float64
}

// nudgeRoutes offsets edges that share corridor segments.
// Uses constraint-based DAG longest-path for optimal spacing,
// with even distribution as fallback.
// The ordering parameter (from Stage 4) determines the order of edges
// within shared corridors to minimize crossings.
func nudgeRoutes(routes []EdgeRoute, channels []Channel, boxes []Rect, ordering *EdgeOrdering) {
	if len(routes) <= 1 {
		return
	}

	// Decompose all routes into segments.
	var allSegs []edgeSegment
	for ri, route := range routes {
		pts := route.Points
		for i := 0; i < len(pts)-1; i++ {
			p1, p2 := pts[i], pts[i+1]
			seg := edgeSegment{edgeIdx: ri, segIdx: i}
			if p1.Y == p2.Y {
				seg.orientation = Horizontal
				seg.fixedCoord = p1.Y
				seg.rangeMin = math.Min(p1.X, p2.X)
				seg.rangeMax = math.Max(p1.X, p2.X)
			} else if p1.X == p2.X {
				seg.orientation = Vertical
				seg.fixedCoord = p1.X
				seg.rangeMin = math.Min(p1.Y, p2.Y)
				seg.rangeMax = math.Max(p1.Y, p2.Y)
			} else {
				continue // diagonal — shouldn't happen
			}
			if seg.rangeMax-seg.rangeMin < 0.5 {
				continue // degenerate
			}
			allSegs = append(allSegs, seg)
		}
	}

	// Group segments into bundles by shared corridor.
	bundles := groupIntoBundles(allSegs)

	// For each bundle with >1 edge, apply constraint-based nudging.
	for _, bundle := range bundles {
		if len(bundle.segments) <= 1 {
			continue
		}

		// Find channel boundaries.
		chMin, chMax := findChannelBounds(bundle, channels)
		channelWidth := chMax - chMin
		if channelWidth < 4 {
			continue // too narrow
		}

		// Unique edges in this bundle.
		edgeSet := make(map[int]bool)
		for _, s := range bundle.segments {
			edgeSet[s.edgeIdx] = true
		}
		uniqueEdges := make([]int, 0, len(edgeSet))
		for e := range edgeSet {
			uniqueEdges = append(uniqueEdges, e)
		}
		// Sort edges using Stage 4 ordering if available, falling back to index order.
		sort.Slice(uniqueEdges, func(i, j int) bool {
			ei, ej := uniqueEdges[i], uniqueEdges[j]
			var ki, kj float64
			if bundle.orientation == Horizontal {
				ki, kj = ordering.HKey(ei), ordering.HKey(ej)
			} else {
				ki, kj = ordering.VKey(ei), ordering.VKey(ej)
			}
			if ki != kj {
				return ki < kj
			}
			return ei < ej // tiebreak by index
		})
		n := len(uniqueEdges)
		if n <= 1 {
			continue
		}

		// Try constraint-based nudging first.
		edgePosition, ok := constraintNudge(uniqueEdges, bundle, chMin, chMax)
		if !ok {
			// Fallback: even distribution.
			edgePosition = evenDistribution(uniqueEdges, bundle.fixedCoord, chMin, channelWidth)
		}

		applyNudgeOffsets(routes, bundle, edgePosition)
	}
}

// constraintNudge builds a constraint DAG and solves via longest-path
// to find optimal positions for edges in a shared corridor.
//
// Returns a map of edge index → offset from current fixedCoord.
// Returns ok=false if the DAG cannot be solved (e.g., cycle detected).
func constraintNudge(uniqueEdges []int, bundle segmentBundle, chMin, chMax float64) (map[int]float64, bool) {
	n := len(uniqueEdges)
	minSpacing := 10.0 // minimum spacing between adjacent parallel edges

	// Sort edges by their current fixed coordinate (or if equal, by edge index).
	// For overlapping horizontal segments, sort by Y; for vertical, sort by X.
	// Since all are at approximately the same fixedCoord, we sort by edge index
	// to get a deterministic order, then apply constraint spacing.

	// Build constraint DAG: edge i must be before edge i+1 with minSpacing.
	// Also add boundary constraints: first edge >= chMin + margin,
	// last edge <= chMax - margin.
	margin := minSpacing / 2

	// Node IDs: 0..n-1 for edges, n for source (chMin), n+1 for sink (chMax).
	numNodes := n + 2
	srcNode := n
	sinkNode := n + 1

	arcs := make([]constraintArc, 0, n+2)

	// Source → first edge: at least margin from channel start.
	arcs = append(arcs, constraintArc{from: srcNode, to: 0, minDist: margin})

	// Chain: edge[i] → edge[i+1] with minSpacing.
	for i := 0; i < n-1; i++ {
		arcs = append(arcs, constraintArc{from: i, to: i + 1, minDist: minSpacing})
	}

	// Last edge → sink: at least margin from channel end.
	arcs = append(arcs, constraintArc{from: n - 1, to: sinkNode, minDist: margin})

	// Solve via topological sort (order is already 0→1→...→n-1→sink, so trivial).
	// Longest-path from source.
	dist := make([]float64, numNodes)
	for i := range dist {
		dist[i] = 0
	}

	// Build adjacency for forward pass.
	adj := make([][]constraintArc, numNodes)
	for _, arc := range arcs {
		adj[arc.from] = append(adj[arc.from], arc)
	}

	// Topological order: srcNode, 0, 1, ..., n-1, sinkNode.
	topoOrder := make([]int, 0, numNodes)
	topoOrder = append(topoOrder, srcNode)
	for i := 0; i < n; i++ {
		topoOrder = append(topoOrder, i)
	}
	topoOrder = append(topoOrder, sinkNode)

	// Longest-path relaxation.
	for _, u := range topoOrder {
		for _, arc := range adj[u] {
			newDist := dist[u] + arc.minDist
			if newDist > dist[arc.to] {
				dist[arc.to] = newDist
			}
		}
	}

	// Check feasibility: the required total width must fit in the channel.
	totalRequired := dist[sinkNode]
	channelWidth := chMax - chMin
	if totalRequired > channelWidth+0.5 {
		// Not enough space: fall back to even distribution.
		return nil, false
	}

	// Center the edge positions within the channel.
	// Available slack = channelWidth - totalRequired.
	slack := channelWidth - totalRequired
	offset := slack / 2 // center the group

	// Compute absolute positions.
	edgePosition := make(map[int]float64, n)
	for i, edgeIdx := range uniqueEdges {
		absPos := chMin + offset + dist[i]
		edgePosition[edgeIdx] = absPos - bundle.fixedCoord
	}

	return edgePosition, true
}

// evenDistribution computes evenly-spaced positions (the fallback strategy).
func evenDistribution(uniqueEdges []int, fixedCoord, chMin, channelWidth float64) map[int]float64 {
	n := len(uniqueEdges)
	edgePosition := make(map[int]float64, n)
	for i, edgeIdx := range uniqueEdges {
		target := chMin + channelWidth*float64(i+1)/float64(n+1)
		edgePosition[edgeIdx] = target - fixedCoord
	}
	return edgePosition
}

// groupIntoBundles groups segments with the same orientation and fixedCoord.
func groupIntoBundles(segs []edgeSegment) []segmentBundle {
	const tolerance = 1.0

	type bundleKey struct {
		orientation Orientation
		fixedBucket int64
	}

	groups := make(map[bundleKey][]edgeSegment)
	for _, s := range segs {
		key := bundleKey{
			orientation: s.orientation,
			fixedBucket: int64(math.Round(s.fixedCoord / tolerance)),
		}
		groups[key] = append(groups[key], s)
	}

	var bundles []segmentBundle
	for key, segs := range groups {
		if len(segs) <= 1 {
			continue
		}
		overlapping := findOverlappingSegments(segs)
		for _, group := range overlapping {
			if len(group) > 1 {
				bundles = append(bundles, segmentBundle{
					orientation: key.orientation,
					fixedCoord:  group[0].fixedCoord,
					segments:    group,
				})
			}
		}
	}

	return bundles
}

// findOverlappingSegments groups segments that overlap in their varying coordinate range.
func findOverlappingSegments(segs []edgeSegment) [][]edgeSegment {
	sort.Slice(segs, func(i, j int) bool {
		return segs[i].rangeMin < segs[j].rangeMin
	})

	var groups [][]edgeSegment
	used := make([]bool, len(segs))

	for i := 0; i < len(segs); i++ {
		if used[i] {
			continue
		}
		group := []edgeSegment{segs[i]}
		used[i] = true
		groupMax := segs[i].rangeMax

		for j := i + 1; j < len(segs); j++ {
			if used[j] {
				continue
			}
			if segs[j].rangeMin < groupMax-0.5 {
				group = append(group, segs[j])
				used[j] = true
				if segs[j].rangeMax > groupMax {
					groupMax = segs[j].rangeMax
				}
			}
		}
		groups = append(groups, group)
	}
	return groups
}

// findChannelBounds returns the min/max of the channel containing the bundle.
// For horizontal segments (shared Y), returns Y range of the channel.
// For vertical segments (shared X), returns X range of the channel.
func findChannelBounds(bundle segmentBundle, channels []Channel) (float64, float64) {
	for _, ch := range channels {
		if bundle.orientation == Horizontal && ch.Orientation == Horizontal {
			if bundle.fixedCoord >= ch.Rect.Top()-1 && bundle.fixedCoord <= ch.Rect.Bottom()+1 {
				return ch.Rect.Top(), ch.Rect.Bottom()
			}
		}
		if bundle.orientation == Vertical && ch.Orientation == Vertical {
			if bundle.fixedCoord >= ch.Rect.Left()-1 && bundle.fixedCoord <= ch.Rect.Right()+1 {
				return ch.Rect.Left(), ch.Rect.Right()
			}
		}
	}
	// Fallback: small spread around current position.
	return bundle.fixedCoord - 10, bundle.fixedCoord + 10
}

// applyNudgeOffsets modifies route points to apply the computed offsets.
// First and last points of each route are port positions on box boundaries
// and must not be moved.
func applyNudgeOffsets(routes []EdgeRoute, bundle segmentBundle, offsets map[int]float64) {
	for _, seg := range bundle.segments {
		offset := offsets[seg.edgeIdx]
		if math.Abs(offset) < 0.1 {
			continue
		}

		pts := routes[seg.edgeIdx].Points
		if seg.segIdx >= len(pts)-1 {
			continue
		}

		// Don't move port endpoints (first and last points).
		isFirstPt := seg.segIdx == 0
		isLastPt := seg.segIdx+1 == len(pts)-1

		if bundle.orientation == Horizontal {
			if !isFirstPt {
				pts[seg.segIdx].Y += offset
			}
			if !isLastPt {
				pts[seg.segIdx+1].Y += offset
			}
		} else {
			if !isFirstPt {
				pts[seg.segIdx].X += offset
			}
			if !isLastPt {
				pts[seg.segIdx+1].X += offset
			}
		}
	}
}
