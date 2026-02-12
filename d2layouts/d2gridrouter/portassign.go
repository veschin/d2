// [FORK] Stage 2: Port Assignment (Hegemann & Wolff §3.2).
// Determines where edges exit/enter cell boundaries.
//
// Algorithm:
// 1. For each edge, draw line between src/dst centers.
// 2. Assign ports to the box sides that line intersects.
// 3. Z-shape avoidance: prefer L-shapes (1 bend) over Z-shapes (2 bends).
// 4. Order ports on each side by circular order of neighbor directions.
// 5. Distribute ports evenly along each side.

package d2gridrouter

import (
	"math"
	"sort"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
)

// assignPorts determines exit/entry ports for all edges.
func assignPorts(boxes []Rect, edges []*d2graph.Edge, nodeIndex map[*d2graph.Object]int) *PortAssignment {
	pa := &PortAssignment{
		SrcPorts: make([]Port, len(edges)),
		DstPorts: make([]Port, len(edges)),
	}

	// Track port count per (node, side) to find least populated sides.
	sideCount := make(map[nodeSideKey]int)

	// Step 1: Determine sides for each edge.
	for i, edge := range edges {
		srcIdx := nodeIndex[edge.Src]
		dstIdx := nodeIndex[edge.Dst]
		srcBox := boxes[srcIdx]
		dstBox := boxes[dstIdx]

		// [FORK] Self-loop: assign neighboring ports on least populated side.
		if edge.Src == edge.Dst {
			srcSide, dstSide := selfLoopSides(srcIdx, sideCount)
			pa.SrcPorts[i] = Port{
				NodeIdx: srcIdx,
				EdgeIdx: i,
				Side:    srcSide,
				IsSrc:   true,
			}
			pa.DstPorts[i] = Port{
				NodeIdx: srcIdx,
				EdgeIdx: i,
				Side:    dstSide,
				IsSrc:   false,
			}
			sideCount[nodeSideKey{srcIdx, srcSide}]++
			sideCount[nodeSideKey{srcIdx, dstSide}]++
			continue
		}

		srcSide, dstSide := determineSides(srcBox, dstBox)

		pa.SrcPorts[i] = Port{
			NodeIdx: srcIdx,
			EdgeIdx: i,
			Side:    srcSide,
			IsSrc:   true,
		}
		pa.DstPorts[i] = Port{
			NodeIdx: dstIdx,
			EdgeIdx: i,
			Side:    dstSide,
			IsSrc:   false,
		}
		sideCount[nodeSideKey{srcIdx, srcSide}]++
		sideCount[nodeSideKey{dstIdx, dstSide}]++
	}

	// Step 2: Order ports on each side and distribute evenly.
	distributePortsOnSides(boxes, pa)

	// Step 3: Align ports for nearly-vertical/horizontal edges to avoid diagonals.
	alignNearlyAlignedPorts(boxes, pa)

	return pa
}

// determineSides decides which side of src and dst boxes an edge should use.
// Uses the angle between centers to pick primary direction, with Z-avoidance.
func determineSides(src, dst Rect) (srcSide, dstSide Direction) {
	srcCx, srcCy := src.CenterX(), src.CenterY()
	dstCx, dstCy := dst.CenterX(), dst.CenterY()

	dx := dstCx - srcCx
	dy := dstCy - srcCy

	// Determine dominant direction based on angle.
	absDx := math.Abs(dx)
	absDy := math.Abs(dy)

	if absDx > absDy {
		// Primarily horizontal.
		if dx > 0 {
			srcSide = DirRight
			dstSide = DirLeft
		} else {
			srcSide = DirLeft
			dstSide = DirRight
		}
		// Z-avoidance: if vertical component is significant (>25% of horizontal),
		// use L-shape instead by adjusting one side to top/bottom.
		if absDy > absDx*0.25 {
			if dy > 0 {
				// Destination is below: prefer exiting src from bottom or entering dst from top.
				// Choose based on which creates an L (not Z).
				dstSide = DirTop
			} else {
				dstSide = DirBottom
			}
		}
	} else if absDy > absDx {
		// Primarily vertical.
		if dy > 0 {
			srcSide = DirBottom
			dstSide = DirTop
		} else {
			srcSide = DirTop
			dstSide = DirBottom
		}
		// Z-avoidance for horizontal component.
		if absDx > absDy*0.25 {
			if dx > 0 {
				dstSide = DirLeft
			} else {
				dstSide = DirRight
			}
		}
	} else {
		// ~45 degrees: use L-shape.
		if dx > 0 {
			srcSide = DirRight
		} else {
			srcSide = DirLeft
		}
		if dy > 0 {
			dstSide = DirTop
		} else {
			dstSide = DirBottom
		}
	}

	return srcSide, dstSide
}

// distributePortsOnSides orders ports on each side and positions them evenly.
func distributePortsOnSides(boxes []Rect, pa *PortAssignment) {
	// Collect all ports per (node, side).
	type nodeSide struct {
		nodeIdx int
		side    Direction
	}
	portsByNodeSide := make(map[nodeSide][]*Port)

	allPorts := make([]*Port, 0, len(pa.SrcPorts)+len(pa.DstPorts))
	for i := range pa.SrcPorts {
		allPorts = append(allPorts, &pa.SrcPorts[i])
	}
	for i := range pa.DstPorts {
		allPorts = append(allPorts, &pa.DstPorts[i])
	}

	for _, p := range allPorts {
		key := nodeSide{p.NodeIdx, p.Side}
		portsByNodeSide[key] = append(portsByNodeSide[key], p)
	}

	// For each (node, side), order ports and distribute evenly.
	for key, ports := range portsByNodeSide {
		box := boxes[key.nodeIdx]

		// Sort ports by the angle to their connected node's center.
		// For a given side, this creates a natural ordering.
		sortPortsByNeighborAngle(ports, boxes, pa)

		// Distribute ports evenly along the side.
		distributeAlongSide(ports, box, key.side)
	}
}

// sortPortsByNeighborAngle sorts ports on a given side by the angle to the
// connected neighbor's center, creating a natural circular ordering.
func sortPortsByNeighborAngle(ports []*Port, boxes []Rect, pa *PortAssignment) {
	sort.Slice(ports, func(i, j int) bool {
		pi := ports[i]
		pj := ports[j]

		// Find the neighbor box for each port.
		var neighborI, neighborJ Rect
		if pi.IsSrc {
			neighborI = boxes[pa.DstPorts[pi.EdgeIdx].NodeIdx]
		} else {
			neighborI = boxes[pa.SrcPorts[pi.EdgeIdx].NodeIdx]
		}
		if pj.IsSrc {
			neighborJ = boxes[pa.DstPorts[pj.EdgeIdx].NodeIdx]
		} else {
			neighborJ = boxes[pa.SrcPorts[pj.EdgeIdx].NodeIdx]
		}

		// Sort by the relevant coordinate of the neighbor's center.
		// For top/bottom sides, sort by X of neighbor.
		// For left/right sides, sort by Y of neighbor.
		switch pi.Side {
		case DirTop, DirBottom:
			return neighborI.CenterX() < neighborJ.CenterX()
		case DirLeft, DirRight:
			return neighborI.CenterY() < neighborJ.CenterY()
		}
		return false
	})
}

// distributeAlongSide positions ports evenly along a box side.
func distributeAlongSide(ports []*Port, box Rect, side Direction) {
	n := len(ports)
	if n == 0 {
		return
	}

	switch side {
	case DirTop:
		// Distribute along top edge (varying X, fixed Y = box.Top).
		for i, p := range ports {
			t := (float64(i) + 1) / (float64(n) + 1)
			p.Pos = geo.Point{X: box.Left() + t*box.W, Y: box.Top()}
		}
	case DirBottom:
		// Distribute along bottom edge.
		for i, p := range ports {
			t := (float64(i) + 1) / (float64(n) + 1)
			p.Pos = geo.Point{X: box.Left() + t*box.W, Y: box.Bottom()}
		}
	case DirLeft:
		// Distribute along left edge (fixed X, varying Y).
		for i, p := range ports {
			t := (float64(i) + 1) / (float64(n) + 1)
			p.Pos = geo.Point{X: box.Left(), Y: box.Top() + t*box.H}
		}
	case DirRight:
		// Distribute along right edge.
		for i, p := range ports {
			t := (float64(i) + 1) / (float64(n) + 1)
			p.Pos = geo.Point{X: box.Right(), Y: box.Top() + t*box.H}
		}
	}
}

// alignNearlyAlignedPorts adjusts port positions for edges where src/dst are
// in the same column or row. Without this, distributed ports create small
// offsets that produce diagonal segments instead of clean straight lines.
func alignNearlyAlignedPorts(boxes []Rect, pa *PortAssignment) {
	for i := range pa.SrcPorts {
		src := &pa.SrcPorts[i]
		dst := &pa.DstPorts[i]
		srcBox := boxes[src.NodeIdx]
		dstBox := boxes[dst.NodeIdx]

		// Vertical edge: top↔bottom with boxes in same column.
		isVertical := (src.Side == DirBottom && dst.Side == DirTop) ||
			(src.Side == DirTop && dst.Side == DirBottom)

		if isVertical {
			// Check if boxes overlap horizontally (same column).
			overlapLeft := math.Max(srcBox.Left(), dstBox.Left())
			overlapRight := math.Min(srcBox.Right(), dstBox.Right())
			if overlapRight > overlapLeft {
				// Boxes share a column. Align X to the midpoint of the overlap,
				// but only if both ports can reach it (within their box bounds).
				targetX := (overlapLeft + overlapRight) / 2
				// Clamp to valid port range on each box.
				srcMinX := srcBox.Left() + srcBox.W*0.1
				srcMaxX := srcBox.Left() + srcBox.W*0.9
				dstMinX := dstBox.Left() + dstBox.W*0.1
				dstMaxX := dstBox.Left() + dstBox.W*0.9
				if targetX >= srcMinX && targetX <= srcMaxX &&
					targetX >= dstMinX && targetX <= dstMaxX {
					src.Pos.X = targetX
					dst.Pos.X = targetX
				}
			}
		}

		// Horizontal edge: left↔right with boxes in same row.
		isHorizontal := (src.Side == DirRight && dst.Side == DirLeft) ||
			(src.Side == DirLeft && dst.Side == DirRight)

		if isHorizontal {
			// Check if boxes overlap vertically (same row).
			overlapTop := math.Max(srcBox.Top(), dstBox.Top())
			overlapBottom := math.Min(srcBox.Bottom(), dstBox.Bottom())
			if overlapBottom > overlapTop {
				targetY := (overlapTop + overlapBottom) / 2
				srcMinY := srcBox.Top() + srcBox.H*0.1
				srcMaxY := srcBox.Top() + srcBox.H*0.9
				dstMinY := dstBox.Top() + dstBox.H*0.1
				dstMaxY := dstBox.Top() + dstBox.H*0.9
				if targetY >= srcMinY && targetY <= srcMaxY &&
					targetY >= dstMinY && targetY <= dstMaxY {
					src.Pos.Y = targetY
					dst.Pos.Y = targetY
				}
			}
		}
	}
}

// nodeSideKey is used to track port counts per (node, side) pair.
type nodeSideKey struct {
	nodeIdx int
	side    Direction
}

// [FORK] selfLoopSides picks two adjacent sides for a self-loop edge.
// It selects the least populated side as primary, then uses the clockwise
// neighbor as the secondary side.
func selfLoopSides(nodeIdx int, sideCount map[nodeSideKey]int) (srcSide, dstSide Direction) {
	sides := []Direction{DirTop, DirRight, DirBottom, DirLeft}

	// Find the side with the fewest ports.
	bestSide := DirRight
	bestCount := math.MaxInt
	for _, s := range sides {
		c := sideCount[nodeSideKey{nodeIdx, s}]
		if c < bestCount {
			bestCount = c
			bestSide = s
		}
	}

	// Use the clockwise neighbor as the second side.
	// Top→Right, Right→Bottom, Bottom→Left, Left→Top.
	nextSide := map[Direction]Direction{
		DirTop:    DirRight,
		DirRight:  DirBottom,
		DirBottom: DirLeft,
		DirLeft:   DirTop,
	}

	return bestSide, nextSide[bestSide]
}
