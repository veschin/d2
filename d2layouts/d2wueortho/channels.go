// [FORK] Stage 3a: Channel Construction (Hegemann & Wolff §3.3).
// Finds routing corridors (channels) between node boxes.
//
// A channel is a maximal empty rectangle between two adjacent boxes
// (or between a box and the drawing boundary). Each channel provides
// a corridor for edge routing.
//
// For a grid layout, channels correspond to the gaps between rows,
// gaps between columns, and the perimeter margins.

package d2wueortho

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

// findChannels discovers all routing channels between boxes.
// Returns both horizontal and vertical channels.
func findChannels(boxes []Rect, bbox Rect) []Channel {
	var channels []Channel

	// For a grid layout, we use a simplified but correct approach:
	// 1. Find all unique row boundaries and column boundaries from boxes.
	// 2. Channels exist in the gaps between adjacent rows/columns.
	// 3. Boundary channels exist at the perimeter.

	// Collect all horizontal and vertical boundaries.
	var xBounds []float64 // left and right edges of all boxes
	var yBounds []float64 // top and bottom edges of all boxes

	for _, b := range boxes {
		xBounds = append(xBounds, b.Left(), b.Right())
		yBounds = append(yBounds, b.Top(), b.Bottom())
	}

	xBounds = append(xBounds, bbox.Left(), bbox.Right())
	yBounds = append(yBounds, bbox.Top(), bbox.Bottom())

	xBounds = sortedUniqueFloats(xBounds)
	yBounds = sortedUniqueFloats(yBounds)

	// Vertical channels: find empty vertical strips between x boundaries.
	// A vertical strip [x1, x2] x [bbox.Top, bbox.Bottom] is a channel
	// if no box overlaps with the interior of the strip.
	for i := 0; i < len(xBounds)-1; i++ {
		x1 := xBounds[i]
		x2 := xBounds[i+1]
		if x2-x1 < 1 {
			continue // too narrow
		}
		midX := (x1 + x2) / 2
		// Check if this vertical strip is free of boxes.
		if isVerticalStripFree(midX, bbox.Top(), bbox.Bottom(), boxes) {
			channels = append(channels, Channel{
				Rect:        Rect{X: x1, Y: bbox.Top(), W: x2 - x1, H: bbox.H},
				Orientation: Vertical,
			})
		}
	}

	// Horizontal channels: find empty horizontal strips between y boundaries.
	for i := 0; i < len(yBounds)-1; i++ {
		y1 := yBounds[i]
		y2 := yBounds[i+1]
		if y2-y1 < 1 {
			continue // too narrow
		}
		midY := (y1 + y2) / 2
		if isHorizontalStripFree(midY, bbox.Left(), bbox.Right(), boxes) {
			channels = append(channels, Channel{
				Rect:        Rect{X: bbox.Left(), Y: y1, W: bbox.W, H: y2 - y1},
				Orientation: Horizontal,
			})
		}
	}

	// [FORK] Prune dominated channels: remove channels whose projection
	// is entirely contained by a wider channel of the same orientation.
	channels = pruneChannels(channels)

	return channels
}

// isVerticalStripFree checks if a vertical line at x is free from all boxes
// between y1 and y2.
func isVerticalStripFree(x, y1, y2 float64, boxes []Rect) bool {
	for _, b := range boxes {
		if x > b.Left() && x < b.Right() {
			// The vertical line passes through this box.
			// Check vertical overlap.
			if y2 > b.Top() && y1 < b.Bottom() {
				return false
			}
		}
	}
	return true
}

// isHorizontalStripFree checks if a horizontal line at y is free from all boxes
// between x1 and x2.
func isHorizontalStripFree(y, x1, x2 float64, boxes []Rect) bool {
	for _, b := range boxes {
		if y > b.Top() && y < b.Bottom() {
			// The horizontal line passes through this box.
			// Check horizontal overlap.
			if x2 > b.Left() && x1 < b.Right() {
				return false
			}
		}
	}
	return true
}

// [FORK] pruneChannels removes dominated channels.
// A channel A is dominated by channel B (same orientation) if A's projection
// along the perpendicular axis is entirely contained in B's, and B is at least
// as wide. In regular grids, this eliminates redundant thin slivers between
// aligned box edges.
func pruneChannels(channels []Channel) []Channel {
	n := len(channels)
	if n <= 1 {
		return channels
	}

	dominated := make([]bool, n)
	for i := 0; i < n; i++ {
		if dominated[i] {
			continue
		}
		for j := 0; j < n; j++ {
			if i == j || dominated[j] {
				continue
			}
			if channels[i].Orientation != channels[j].Orientation {
				continue
			}
			if dominates(channels[i], channels[j]) {
				dominated[j] = true
			}
		}
	}

	var result []Channel
	for i, ch := range channels {
		if !dominated[i] {
			result = append(result, ch)
		}
	}
	return result
}

// dominates returns true if channel A dominates channel B (B is redundant).
// Both must have the same orientation.
// For vertical channels: A dominates B if A's X-range contains B's X-range
// and A's height (perpendicular extent) >= B's height.
// For horizontal channels: A dominates B if A's Y-range contains B's Y-range
// and A's width (perpendicular extent) >= B's width.
func dominates(a, b Channel) bool {
	const eps = 0.5
	if a.Orientation == Vertical {
		// "Width" of a vertical channel = its X extent.
		// "Projection" = its Y extent (perpendicular).
		// A dominates B if A contains B's X-range and A is at least as tall.
		aContainsX := a.Rect.Left()-eps <= b.Rect.Left() && a.Rect.Right()+eps >= b.Rect.Right()
		aAtLeastAsTall := a.Rect.Top()-eps <= b.Rect.Top() && a.Rect.Bottom()+eps >= b.Rect.Bottom()
		aWider := a.Rect.W > b.Rect.W+eps
		return aContainsX && aAtLeastAsTall && aWider
	}
	// Horizontal.
	aContainsY := a.Rect.Top()-eps <= b.Rect.Top() && a.Rect.Bottom()+eps >= b.Rect.Bottom()
	aAtLeastAsWide := a.Rect.Left()-eps <= b.Rect.Left() && a.Rect.Right()+eps >= b.Rect.Right()
	aTaller := a.Rect.H > b.Rect.H+eps
	return aContainsY && aAtLeastAsWide && aTaller
}

// buildRepresentatives creates representative line segments for each channel.
// Vertical channels get vertical representative lines (at channel center X).
// Horizontal channels get horizontal representative lines (at channel center Y).
func buildRepresentatives(channels []Channel, ports *PortAssignment) []Segment {
	var segments []Segment

	for _, ch := range channels {
		if ch.Orientation == Vertical {
			// Vertical channel → vertical representative line at center X.
			centerX := ch.Rect.CenterX()

			// Check if any port aligns with this channel; prefer port-aligned.
			alignedX := centerX
			bestDist := ch.Rect.W / 2
			allPorts := make([]Port, 0, len(ports.SrcPorts)+len(ports.DstPorts))
			allPorts = append(allPorts, ports.SrcPorts...)
			allPorts = append(allPorts, ports.DstPorts...)
			for _, p := range allPorts {
				if p.Pos.X > ch.Rect.Left() && p.Pos.X < ch.Rect.Right() {
					dist := math.Abs(p.Pos.X - centerX)
					if dist < bestDist {
						alignedX = p.Pos.X
						bestDist = dist
					}
				}
			}

			segments = append(segments, Segment{
				Start:       geo.Point{X: alignedX, Y: ch.Rect.Top()},
				End:         geo.Point{X: alignedX, Y: ch.Rect.Bottom()},
				Orientation: Vertical,
			})
		} else {
			// Horizontal channel → horizontal representative line at center Y.
			centerY := ch.Rect.CenterY()

			alignedY := centerY
			bestDist := ch.Rect.H / 2
			allPorts := make([]Port, 0, len(ports.SrcPorts)+len(ports.DstPorts))
			allPorts = append(allPorts, ports.SrcPorts...)
			allPorts = append(allPorts, ports.DstPorts...)
			for _, p := range allPorts {
				if p.Pos.Y > ch.Rect.Top() && p.Pos.Y < ch.Rect.Bottom() {
					dist := math.Abs(p.Pos.Y - centerY)
					if dist < bestDist {
						alignedY = p.Pos.Y
						bestDist = dist
					}
				}
			}

			segments = append(segments, Segment{
				Start:       geo.Point{X: ch.Rect.Left(), Y: alignedY},
				End:         geo.Point{X: ch.Rect.Right(), Y: alignedY},
				Orientation: Horizontal,
			})
		}
	}

	// Add port-aligned segments for ports not covered by any channel representative.
	segments = addPortSegments(segments, ports, channels)

	return segments
}

// addPortSegments adds short segments through ports that don't align with any
// existing representative.
func addPortSegments(segments []Segment, ports *PortAssignment, channels []Channel) []Segment {
	allPorts := make([]Port, 0, len(ports.SrcPorts)+len(ports.DstPorts))
	allPorts = append(allPorts, ports.SrcPorts...)
	allPorts = append(allPorts, ports.DstPorts...)

	for _, p := range allPorts {
		covered := false
		for _, s := range segments {
			if s.Orientation == Vertical && s.Start.X == p.Pos.X {
				if p.Pos.Y >= s.Start.Y && p.Pos.Y <= s.End.Y {
					covered = true
					break
				}
			}
			if s.Orientation == Horizontal && s.Start.Y == p.Pos.Y {
				if p.Pos.X >= s.Start.X && p.Pos.X <= s.End.X {
					covered = true
					break
				}
			}
		}
		if !covered {
			// Add a segment through this port extending into the nearest channel.
			// For ports on top/bottom, add vertical segment.
			// For ports on left/right, add horizontal segment.
			if p.Side == DirTop || p.Side == DirBottom {
				// Find the vertical extent we can go.
				seg := extendVerticalFromPort(p, channels)
				if seg != nil {
					segments = append(segments, *seg)
				}
			} else {
				seg := extendHorizontalFromPort(p, channels)
				if seg != nil {
					segments = append(segments, *seg)
				}
			}
		}
	}

	return segments
}

// extendVerticalFromPort creates a vertical segment from a port into the
// adjacent horizontal channel.
func extendVerticalFromPort(p Port, channels []Channel) *Segment {
	// Find horizontal channels adjacent to this port.
	for _, ch := range channels {
		if ch.Orientation != Horizontal {
			continue
		}
		// Check if the port's X is within the channel's horizontal span.
		if p.Pos.X >= ch.Rect.Left() && p.Pos.X <= ch.Rect.Right() {
			// Check adjacency.
			if p.Side == DirTop && ch.Rect.Bottom() <= p.Pos.Y+1 {
				return &Segment{
					Start:       geo.Point{X: p.Pos.X, Y: ch.Rect.CenterY()},
					End:         geo.Point{X: p.Pos.X, Y: p.Pos.Y},
					Orientation: Vertical,
				}
			}
			if p.Side == DirBottom && ch.Rect.Top() >= p.Pos.Y-1 {
				return &Segment{
					Start:       geo.Point{X: p.Pos.X, Y: p.Pos.Y},
					End:         geo.Point{X: p.Pos.X, Y: ch.Rect.CenterY()},
					Orientation: Vertical,
				}
			}
		}
	}
	return nil
}

// extendHorizontalFromPort creates a horizontal segment from a port into the
// adjacent vertical channel.
func extendHorizontalFromPort(p Port, channels []Channel) *Segment {
	for _, ch := range channels {
		if ch.Orientation != Vertical {
			continue
		}
		if p.Pos.Y >= ch.Rect.Top() && p.Pos.Y <= ch.Rect.Bottom() {
			if p.Side == DirLeft && ch.Rect.Right() <= p.Pos.X+1 {
				return &Segment{
					Start:       geo.Point{X: ch.Rect.CenterX(), Y: p.Pos.Y},
					End:         geo.Point{X: p.Pos.X, Y: p.Pos.Y},
					Orientation: Horizontal,
				}
			}
			if p.Side == DirRight && ch.Rect.Left() >= p.Pos.X-1 {
				return &Segment{
					Start:       geo.Point{X: p.Pos.X, Y: p.Pos.Y},
					End:         geo.Point{X: ch.Rect.CenterX(), Y: p.Pos.Y},
					Orientation: Horizontal,
				}
			}
		}
	}
	return nil
}

// sortSegmentPoints ensures Start <= End for each segment.
func sortSegmentPoints(segments []Segment) {
	for i := range segments {
		s := &segments[i]
		if s.Orientation == Horizontal {
			if s.Start.X > s.End.X {
				s.Start, s.End = s.End, s.Start
			}
		} else {
			if s.Start.Y > s.End.Y {
				s.Start, s.End = s.End, s.Start
			}
		}
	}
}

// deduplicateSegments removes duplicate segments with same start/end.
func deduplicateSegments(segments []Segment) []Segment {
	type segKey struct {
		sx, sy, ex, ey float64
		o              Orientation
	}
	seen := make(map[segKey]bool)
	var result []Segment
	for _, s := range segments {
		k := segKey{s.Start.X, s.Start.Y, s.End.X, s.End.Y, s.Orientation}
		if !seen[k] {
			seen[k] = true
			result = append(result, s)
		}
	}
	return result
}
