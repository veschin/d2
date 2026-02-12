// [FORK] This file is added by the fork for the wueortho layout engine.
// Implements L/Z-Router for standalone mode: grid-aware orthogonal edge routing
// with bends near target entry, port spreading, and obstacle avoidance.
//
// Route types:
// - Straight: same row/col, adjacent cells → 2 points, 0 bends
// - L-route:  diagonal cells → 3 points, 1 bend near target entry
// - Z-route:  when L crosses occupied cell → 4 points, 2 bends through channel

package d2wueortho

import (
	"math"
	"sort"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
)

// face represents a side of a node's bounding box.
type face int

const (
	faceRight  face = iota
	faceLeft
	faceTop
	faceBottom
)

// portInfo holds the computed port position and metadata for one edge endpoint.
type portInfo struct {
	edge    *d2graph.Edge
	obj     *d2graph.Object
	face    face
	pos     *geo.Point // computed port position on the shape boundary
	isSource bool
}

// gridRouteEdges routes all edges in g using grid-aware L/Z routing.
// It uses the grid placement info to determine faces, assign ports, and construct routes.
func gridRouteEdges(g *d2graph.Graph, info *gridInfo) {
	if len(g.Edges) == 0 || info == nil || len(info.placement) == 0 {
		return
	}

	// Build object index for looking up grid cells.
	objIndex := make(map[*d2graph.Object]int, len(info.objects))
	for i, obj := range info.objects {
		objIndex[obj] = i
	}

	// Step 1: Determine face for each edge endpoint.
	// Two-pass: mandatory faces first, then load-balanced faces for equal diagonals.
	type edgeFaces struct {
		srcFace, dstFace face
	}
	edgeFaceMap := make(map[*d2graph.Edge]edgeFaces, len(g.Edges))

	type nfKey struct {
		idx int
		f   face
	}
	faceLoad := make(map[nfKey]int)

	// Pass 1: Edges with clear face preference (same row/col, strictly dominant diagonal).
	var flexEdges []*d2graph.Edge
	for _, e := range g.Edges {
		si, sok := objIndex[e.Src]
		di, dok := objIndex[e.Dst]
		if !sok || !dok {
			continue
		}
		sc, dc := info.placement[si], info.placement[di]
		dcol := dc.col - sc.col
		drow := dc.row - sc.row

		if dcol != 0 && absInt(dcol) == absInt(drow) {
			flexEdges = append(flexEdges, e)
			continue
		}

		sf, df := selectFaces(sc, dc)
		edgeFaceMap[e] = edgeFaces{sf, df}
		faceLoad[nfKey{si, sf}]++
		faceLoad[nfKey{di, df}]++
	}

	// Pass 2: Equal diagonals — each endpoint independently picks the face
	// with lower current load. This creates mixed face pairs (e.g., src exits
	// BOTTOM while dst enters LEFT) which push L-route bends to the layout
	// corners, keeping the center clean. Tiebreaker: prefer vertical faces.
	for _, e := range flexEdges {
		si := objIndex[e.Src]
		di := objIndex[e.Dst]
		sc, dc := info.placement[si], info.placement[di]
		dcol := dc.col - sc.col
		drow := dc.row - sc.row

		// Candidate faces for source and destination on each axis.
		var srcV, srcH, dstV, dstH face
		if drow > 0 {
			srcV, dstV = faceBottom, faceTop
		} else {
			srcV, dstV = faceTop, faceBottom
		}
		if dcol > 0 {
			srcH, dstH = faceRight, faceLeft
		} else {
			srcH, dstH = faceLeft, faceRight
		}

		// Source picks the face with lower load (vertical wins ties).
		sf := srcV
		if faceLoad[nfKey{si, srcH}] < faceLoad[nfKey{si, srcV}] {
			sf = srcH
		}
		// Destination picks the face with lower load (vertical wins ties).
		df := dstV
		if faceLoad[nfKey{di, dstH}] < faceLoad[nfKey{di, dstV}] {
			df = dstH
		}

		edgeFaceMap[e] = edgeFaces{sf, df}
		faceLoad[nfKey{si, sf}]++
		faceLoad[nfKey{di, df}]++
	}

	// Step 2: Group edges by (node, face) and assign port positions.
	type nodeFaceKey struct {
		nodeIdx int
		f       face
	}
	faceEdges := make(map[nodeFaceKey][]*portInfo)

	for _, e := range g.Edges {
		si, sok := objIndex[e.Src]
		di, dok := objIndex[e.Dst]
		if !sok || !dok {
			continue
		}
		faces := edgeFaceMap[e]

		srcPort := &portInfo{edge: e, obj: e.Src, face: faces.srcFace, isSource: true}
		dstPort := &portInfo{edge: e, obj: e.Dst, face: faces.dstFace, isSource: false}

		faceEdges[nodeFaceKey{si, faces.srcFace}] = append(faceEdges[nodeFaceKey{si, faces.srcFace}], srcPort)
		faceEdges[nodeFaceKey{di, faces.dstFace}] = append(faceEdges[nodeFaceKey{di, faces.dstFace}], dstPort)
	}

	// Sort ports on each face by neighbor position and assign coordinates.
	for key, ports := range faceEdges {
		sortAndSpreadPorts(ports, key.f, info, objIndex)
	}

	// Step 2b: Align ports for straight edges (same row/col, adjacent).
	// After independent spreading, ports on src and dst may have different X (for vertical)
	// or Y (for horizontal), creating diagonal lines. Align them to the midpoint.
	for _, e := range g.Edges {
		si, sok := objIndex[e.Src]
		di, dok := objIndex[e.Dst]
		if !sok || !dok {
			continue
		}
		sc, dc := info.placement[si], info.placement[di]
		faces := edgeFaceMap[e]

		// Find the portInfo for this edge's src and dst.
		var srcPI, dstPI *portInfo
		for _, ports := range faceEdges {
			for _, p := range ports {
				if p.edge == e && p.isSource {
					srcPI = p
				}
				if p.edge == e && !p.isSource {
					dstPI = p
				}
			}
		}
		if srcPI == nil || dstPI == nil || srcPI.pos == nil || dstPI.pos == nil {
			continue
		}

		// Same column, vertical faces (BOTTOM/TOP or TOP/BOTTOM): align X.
		if sc.col == dc.col && absInt(sc.row-dc.row) == 1 {
			if (faces.srcFace == faceBottom && faces.dstFace == faceTop) ||
				(faces.srcFace == faceTop && faces.dstFace == faceBottom) {
				// Align to the port on the face with FEWER ports (it's centered).
				// The multi-port face adjusts its port to match.
				srcKey := nodeFaceKey{si, faces.srcFace}
				dstKey := nodeFaceKey{di, faces.dstFace}
				srcCount := len(faceEdges[srcKey])
				dstCount := len(faceEdges[dstKey])
				var alignX float64
				if srcCount <= dstCount {
					alignX = srcPI.pos.X // src has fewer ports → centered, keep it
				} else {
					alignX = dstPI.pos.X // dst has fewer ports → centered, keep it
				}
				srcPI.pos = geo.NewPoint(alignX, srcPI.pos.Y)
				dstPI.pos = geo.NewPoint(alignX, dstPI.pos.Y)
			}
		}

		// Same row, horizontal faces (RIGHT/LEFT or LEFT/RIGHT): align Y.
		if sc.row == dc.row && absInt(sc.col-dc.col) == 1 {
			if (faces.srcFace == faceRight && faces.dstFace == faceLeft) ||
				(faces.srcFace == faceLeft && faces.dstFace == faceRight) {
				srcKey := nodeFaceKey{si, faces.srcFace}
				dstKey := nodeFaceKey{di, faces.dstFace}
				srcCount := len(faceEdges[srcKey])
				dstCount := len(faceEdges[dstKey])
				var alignY float64
				if srcCount <= dstCount {
					alignY = srcPI.pos.Y // src has fewer → centered, keep it
				} else {
					alignY = dstPI.pos.Y // dst has fewer → centered, keep it
				}
				srcPI.pos = geo.NewPoint(srcPI.pos.X, alignY)
				dstPI.pos = geo.NewPoint(dstPI.pos.X, alignY)
			}
		}
	}

	// Build port lookup: edge → (srcPort, dstPort).
	type portPair struct {
		src, dst *geo.Point
	}
	edgePorts := make(map[*d2graph.Edge]portPair, len(g.Edges))
	for _, ports := range faceEdges {
		for _, p := range ports {
			pp := edgePorts[p.edge]
			if p.isSource {
				pp.src = p.pos
			} else {
				pp.dst = p.pos
			}
			edgePorts[p.edge] = pp
		}
	}

	// Step 3: Construct routes for each edge.
	for _, e := range g.Edges {
		pp, ok := edgePorts[e]
		if !ok || pp.src == nil || pp.dst == nil {
			// Fallback: center-to-center straight line.
			e.Route = []*geo.Point{e.Src.Center(), e.Dst.Center()}
			continue
		}

		si := objIndex[e.Src]
		di := objIndex[e.Dst]
		faces := edgeFaceMap[e]

		route := constructRoute(pp.src, pp.dst, faces.srcFace, faces.dstFace, info.placement[si], info.placement[di], info)
		e.Route = route
		e.IsCurve = false
	}
}

// selectFaces determines exit face for src and entry face for dst based on grid positions.
func selectFaces(srcCell, dstCell gridCell) (srcFace, dstFace face) {
	dc := dstCell.col - srcCell.col
	dr := dstCell.row - srcCell.row

	if dr == 0 && dc == 0 {
		// Self-loop or same cell: use right/left.
		return faceRight, faceLeft
	}

	if dr == 0 {
		// Same row.
		if dc > 0 {
			return faceRight, faceLeft
		}
		return faceLeft, faceRight
	}

	if dc == 0 {
		// Same column.
		if dr > 0 {
			return faceBottom, faceTop
		}
		return faceTop, faceBottom
	}

	// Diagonal: dominant axis determines faces.
	// When axes are equal (|dc|==|dr|), prefer vertical faces — this distributes
	// ports across more faces (avoiding congestion on horizontal faces from same-row edges)
	// and produces cleaner L-routes that match the natural flow direction.
	if absInt(dc) > absInt(dr) {
		// Horizontal strictly dominant.
		if dc > 0 {
			return faceRight, faceLeft
		}
		return faceLeft, faceRight
	}
	// Vertical dominant or equal.
	if dr > 0 {
		return faceBottom, faceTop
	}
	return faceTop, faceBottom
}

// sortAndSpreadPorts sorts ports on a face by neighbor position and assigns evenly-spaced coordinates.
func sortAndSpreadPorts(ports []*portInfo, f face, info *gridInfo, objIndex map[*d2graph.Object]int) {
	if len(ports) == 0 {
		return
	}

	// Sort by the position of the OTHER endpoint (the neighbor).
	sort.Slice(ports, func(i, j int) bool {
		ni := neighborObj(ports[i])
		nj := neighborObj(ports[j])
		switch f {
		case faceTop, faceBottom:
			// Sort by neighbor's X (column position).
			return ni.Center().X < nj.Center().X
		case faceLeft, faceRight:
			// Sort by neighbor's Y (row position).
			return ni.Center().Y < nj.Center().Y
		}
		return false
	})

	// Distribute ports evenly along the face.
	obj := ports[0].obj
	n := len(ports)
	minClearance := 8.0  // min gap between adjacent ports
	cornerGap := 12.0    // min gap from face corners

	for i, p := range ports {
		t := (float64(i) + 1) / (float64(n) + 1)
		p.pos = facePoint(obj, f, t, cornerGap, minClearance, n)
	}
}

// neighborObj returns the OTHER endpoint of this port's edge.
func neighborObj(p *portInfo) *d2graph.Object {
	if p.isSource {
		return p.edge.Dst
	}
	return p.edge.Src
}

// facePoint computes a point on a face at parameter t ∈ (0,1).
// Respects cornerGap and minClearance constraints.
func facePoint(obj *d2graph.Object, f face, t float64, cornerGap, minClearance float64, numPorts int) *geo.Point {
	box := obj.Box
	left := box.TopLeft.X
	top := box.TopLeft.Y
	right := left + obj.Width
	bottom := top + obj.Height

	switch f {
	case faceTop:
		usable := obj.Width - 2*cornerGap
		if usable < 0 {
			usable = obj.Width
			cornerGap = 0
		}
		x := left + cornerGap + usable*t
		return geo.NewPoint(x, top)
	case faceBottom:
		usable := obj.Width - 2*cornerGap
		if usable < 0 {
			usable = obj.Width
			cornerGap = 0
		}
		x := left + cornerGap + usable*t
		return geo.NewPoint(x, bottom)
	case faceLeft:
		usable := obj.Height - 2*cornerGap
		if usable < 0 {
			usable = obj.Height
			cornerGap = 0
		}
		y := top + cornerGap + usable*t
		return geo.NewPoint(left, y)
	case faceRight:
		usable := obj.Height - 2*cornerGap
		if usable < 0 {
			usable = obj.Height
			cornerGap = 0
		}
		y := top + cornerGap + usable*t
		return geo.NewPoint(right, y)
	}
	return obj.Center()
}

// constructRoute builds an orthogonal route from srcPort to dstPort.
// Uses straight/L/Z routing depending on geometry and obstacles.
//
// Priority: fewer bends first. L-route orientation matches source exit direction.
//  1. Straight line (0 bends)
//  2. L-route matching srcFace exit direction (1 bend)
//  3. L-route alternative orientation (1 bend)
//  4. Z-route through channel (2 bends)
//  5. Z-route perpendicular/opposite fallbacks (2 bends)
func constructRoute(srcPort, dstPort *geo.Point, srcFace, dstFace face, srcCell, dstCell gridCell, info *gridInfo) []*geo.Point {
	// Same row or same column → try straight line first.
	if srcCell.row == dstCell.row || srcCell.col == dstCell.col {
		straight := []*geo.Point{srcPort, dstPort}
		if !routeCrossesNode(straight, info, srcCell, dstCell) {
			return straight
		}
	}

	// L-route: try the orientation that matches the source exit direction first.
	// Vertical exit (TOP/BOTTOM) → first segment goes vertical: bend at (src.X, dst.Y).
	// Horizontal exit (LEFT/RIGHT) → first segment goes horizontal: bend at (dst.X, src.Y).
	var bendPrimary, bendAlt *geo.Point
	if srcFace == faceTop || srcFace == faceBottom {
		bendPrimary = geo.NewPoint(srcPort.X, dstPort.Y) // vertical first
		bendAlt = geo.NewPoint(dstPort.X, srcPort.Y)     // horizontal first
	} else {
		bendPrimary = geo.NewPoint(dstPort.X, srcPort.Y) // horizontal first
		bendAlt = geo.NewPoint(srcPort.X, dstPort.Y)     // vertical first
	}

	lPrimary := []*geo.Point{srcPort, bendPrimary, dstPort}
	if !routeCrossesNode(lPrimary, info, srcCell, dstCell) {
		return lPrimary
	}

	lAlt := []*geo.Point{srcPort, bendAlt, dstPort}
	if !routeCrossesNode(lAlt, info, srcCell, dstCell) {
		return lAlt
	}

	// Z-route: two bends through channel (last resort).
	zRoute := buildZRoute(srcPort, dstPort, dstFace, srcCell, dstCell, info)
	if !routeCrossesNode(zRoute, info, srcCell, dstCell) {
		return zRoute
	}

	// Z-route perpendicular fallbacks.
	perpFace := perpendicularFace(dstFace)
	zRoutePerp := buildZRoute(srcPort, dstPort, perpFace, srcCell, dstCell, info)
	if !routeCrossesNode(zRoutePerp, info, srcCell, dstCell) {
		return zRoutePerp
	}

	perpFace2 := oppositeFace(perpFace)
	zRoutePerp2 := buildZRoute(srcPort, dstPort, perpFace2, srcCell, dstCell, info)
	if !routeCrossesNode(zRoutePerp2, info, srcCell, dstCell) {
		return zRoutePerp2
	}

	return zRoute
}

// perpendicularFace returns a face perpendicular to the given face.
func perpendicularFace(f face) face {
	switch f {
	case faceLeft, faceRight:
		return faceTop
	case faceTop, faceBottom:
		return faceRight
	}
	return f
}

// buildZRoute creates a 4-point Z-shaped route through the channel between cells.
func buildZRoute(src, dst *geo.Point, dstFace face, srcCell, dstCell gridCell, info *gridInfo) []*geo.Point {
	// Determine channel position: midpoint between the two rows or columns.
	switch dstFace {
	case faceTop, faceBottom:
		// Route horizontally through a channel between rows.
		// Try channel above and below, pick less obstructed.
		channelY := findHorizontalChannel(src.Y, dst.Y, srcCell, dstCell, info)
		p1 := geo.NewPoint(src.X, channelY)
		p2 := geo.NewPoint(dst.X, channelY)
		return []*geo.Point{src, p1, p2, dst}
	case faceLeft, faceRight:
		// Route vertically through a channel between columns.
		channelX := findVerticalChannel(src.X, dst.X, srcCell, dstCell, info)
		p1 := geo.NewPoint(channelX, src.Y)
		p2 := geo.NewPoint(channelX, dst.Y)
		return []*geo.Point{src, p1, p2, dst}
	}
	// Fallback.
	return []*geo.Point{src, dst}
}

// findHorizontalChannel finds a Y-coordinate for a horizontal channel segment.
// Uses the gap between node bounding boxes in adjacent rows (the channel region).
func findHorizontalChannel(srcY, dstY float64, srcCell, dstCell gridCell, info *gridInfo) float64 {
	// Use the boundary between the two rows as the channel.
	minR := srcCell.row
	maxR := dstCell.row
	if minR > maxR {
		minR, maxR = maxR, minR
	}

	// Channel at the boundary between row minR and row minR+1.
	if minR != maxR {
		channelY := info.rowY[minR] + info.rowHeight[minR]
		return channelY
	}

	// Same row: route through the gap between this row and an adjacent row.
	// The row boundary is naturally in the center of the channel gap between nodes.
	r := srcCell.row
	aboveY := info.rowY[r]                       // top boundary of this row = bottom of gap above
	belowY := info.rowY[r] + info.rowHeight[r]   // bottom boundary of this row = top of gap below

	if math.Abs(srcY-aboveY) < math.Abs(srcY-belowY) {
		return aboveY // row boundary above (in the gap between nodes)
	}
	return belowY // row boundary below (in the gap between nodes)
}

// findVerticalChannel finds an X-coordinate for a vertical channel segment.
// Uses the gap between node bounding boxes in adjacent columns.
func findVerticalChannel(srcX, dstX float64, srcCell, dstCell gridCell, info *gridInfo) float64 {
	minC := srcCell.col
	maxC := dstCell.col
	if minC > maxC {
		minC, maxC = maxC, minC
	}

	if minC != maxC {
		channelX := info.colX[minC] + info.colWidth[minC]
		return channelX
	}

	// Same column: route through the gap at the column boundary.
	c := srcCell.col
	leftX := info.colX[c]                       // left boundary of this column
	rightX := info.colX[c] + info.colWidth[c]   // right boundary of this column

	if math.Abs(srcX-leftX) < math.Abs(srcX-rightX) {
		return leftX
	}
	return rightX
}

// routeCrossesNode checks if any segment of the route passes through an occupied cell's bounding box.
// Ignores the source and destination cells themselves.
func routeCrossesNode(route []*geo.Point, info *gridInfo, srcCell, dstCell gridCell) bool {
	for cell, nodeIdx := range info.occupied {
		if cell == srcCell || cell == dstCell {
			continue
		}
		obj := info.objects[nodeIdx]
		box := nodeBox(obj, 4.0) // 4px margin
		for i := 0; i < len(route)-1; i++ {
			if segmentIntersectsBox(route[i], route[i+1], box) {
				return true
			}
		}
	}
	return false
}

// nodeBox returns the bounding box of a node with a margin.
func nodeBox(obj *d2graph.Object, margin float64) [4]float64 {
	return [4]float64{
		obj.TopLeft.X - margin,             // left
		obj.TopLeft.Y - margin,             // top
		obj.TopLeft.X + obj.Width + margin,  // right
		obj.TopLeft.Y + obj.Height + margin, // bottom
	}
}

// segmentIntersectsBox checks if a line segment from p1 to p2 intersects an AABB [left, top, right, bottom].
// Uses Liang-Barsky algorithm for correct handling of arbitrary segment orientations.
func segmentIntersectsBox(p1, p2 *geo.Point, box [4]float64) bool {
	left, top, right, bottom := box[0], box[1], box[2], box[3]

	// Quick bounding-box rejection.
	minX := math.Min(p1.X, p2.X)
	maxX := math.Max(p1.X, p2.X)
	minY := math.Min(p1.Y, p2.Y)
	maxY := math.Max(p1.Y, p2.Y)
	if maxX < left || minX > right || maxY < top || minY > bottom {
		return false
	}

	// For orthogonal segments (H or V), the bounding box check is sufficient.
	if math.Abs(p1.X-p2.X) < 0.5 || math.Abs(p1.Y-p2.Y) < 0.5 {
		return true
	}

	// General case: Liang-Barsky line clipping.
	dx := p2.X - p1.X
	dy := p2.Y - p1.Y
	tMin := 0.0
	tMax := 1.0

	clip := func(p, q float64) bool {
		if p == 0 {
			return q >= 0
		}
		t := q / p
		if p < 0 {
			if t > tMax {
				return false
			}
			if t > tMin {
				tMin = t
			}
		} else {
			if t < tMin {
				return false
			}
			if t < tMax {
				tMax = t
			}
		}
		return true
	}

	return clip(-dx, p1.X-left) &&
		clip(dx, right-p1.X) &&
		clip(-dy, p1.Y-top) &&
		clip(dy, bottom-p1.Y) &&
		tMin <= tMax
}

func oppositeFace(f face) face {
	switch f {
	case faceTop:
		return faceBottom
	case faceBottom:
		return faceTop
	case faceLeft:
		return faceRight
	case faceRight:
		return faceLeft
	}
	return f
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
