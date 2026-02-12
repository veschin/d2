// [FORK] Stage 5: Simplified Nudging (Hegemann & Wolff §3.5).
// Separates edges that share corridor segments by distributing them
// evenly across the full channel width.
//
// For N edges sharing a corridor of width W, each edge gets position
// W * (i+1)/(N+1), measured from the channel boundary. This gives
// equal spacing between edges and between edges and channel walls.

package d2gridrouter

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

// nudgeRoutes offsets edges that share corridor segments so they run parallel,
// evenly dividing the corridor width.
func nudgeRoutes(routes []EdgeRoute, channels []Channel, boxes []Rect) {
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

	// For each bundle with >1 edge, distribute evenly across channel.
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
		sort.Ints(uniqueEdges)
		n := len(uniqueEdges)
		if n <= 1 {
			continue
		}

		// Compute target position for each edge: evenly spaced across channel.
		// Position i = chMin + channelWidth * (i+1) / (n+1)
		// Offset = target - current fixedCoord
		edgePosition := make(map[int]float64)
		for i, edgeIdx := range uniqueEdges {
			target := chMin + channelWidth*float64(i+1)/float64(n+1)
			edgePosition[edgeIdx] = target - bundle.fixedCoord
		}

		applyNudgeOffsets(routes, bundle, edgePosition)
	}
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
