// [FORK] This file is added by the fork for the wueortho layout engine.
// Implements the full Hegemann & Wolff (2023) pipeline as a standalone layout engine.
//
// Stage 1: Grid-snap BFS node placement — places nodes on a virtual grid using
// BFS traversal from the most connected node, producing aligned, compact layouts.
// Stage 2: L/Z orthogonal edge routing (gridroute.go) for standalone mode.
// Stages 2-5: Dijkstra-based routing (router.go) for grid routing mode (ELK/dagre).

package d2wueortho

import (
	"context"
	"math"
	"sort"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
)

// gridCell represents a position in the virtual grid.
type gridCell struct {
	row, col int
}

// gridInfo holds the result of grid placement for use by the router.
type gridInfo struct {
	placement map[int]gridCell // node index → grid cell
	occupied  map[gridCell]int // grid cell → node index (-1 if none)
	colWidth  map[int]float64  // column index → pixel width
	rowHeight map[int]float64  // row index → pixel height
	colX      map[int]float64  // column index → pixel X start
	rowY      map[int]float64  // row index → pixel Y start
	objects   []*d2graph.Object
	channel   float64
}

// Layout positions nodes on a virtual grid and routes edges orthogonally.
// This is the entry point for wueortho as a standalone layout engine.
func Layout(ctx context.Context, g *d2graph.Graph, opts *ConfigurableOpts) error {
	if opts == nil {
		opts = &DefaultOpts
	}
	if len(g.Objects) == 0 {
		return nil
	}

	// Stage 1: Place nodes on a virtual grid using BFS from most-connected node.
	info := gridPlacement(g, opts)

	// Position labels (required for correct SVG rendering).
	positionLabels(g)

	// Stage 2: L/Z orthogonal edge routing for standalone mode.
	if len(g.Edges) > 0 {
		gridRouteEdges(g, info)
	}

	// Position edge labels: offset above the edge so they don't overlap the line.
	for _, e := range g.Edges {
		if e.Label.Value != "" && e.LabelPosition == nil {
			e.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
		}
	}

	return nil
}

// positionLabels sets label and icon positions on all objects, matching dagre behavior.
// Without explicit LabelPosition, the SVG renderer places text at the top-left corner.
func positionLabels(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		// Position icons first (same logic as dagre's positionLabelsIcons).
		if obj.Icon != nil && obj.IconPosition == nil {
			if len(obj.ChildrenArray) > 0 {
				obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
				if obj.LabelPosition == nil {
					obj.LabelPosition = go2.Pointer(label.OutsideTopRight.String())
				}
			} else if obj.SQLTable != nil || obj.Class != nil || obj.Language != "" {
				obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
			} else {
				obj.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
			}
		}

		if !obj.HasLabel() || obj.LabelPosition != nil {
			continue
		}

		if len(obj.ChildrenArray) > 0 {
			obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
		} else if obj.HasOutsideBottomLabel() {
			// Image and person shapes: label goes below the shape.
			obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
		} else if obj.Icon != nil {
			obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
		} else {
			obj.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}

		// If label overflows the shape, move it outside.
		if float64(obj.LabelDimensions.Width) > obj.Width || float64(obj.LabelDimensions.Height) > obj.Height {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
			} else {
				obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
			}
		}
	}
}

// gridPlacement places nodes on a virtual grid using BFS from the most-connected node.
// Returns gridInfo with placement data for the edge router.
//
// Improvements over naive BFS:
// - Variable cell sizes (per-row/per-column) to avoid wasting space
// - Respects graph direction attribute for BFS expansion order
// - Edge-direction-aware neighbor placement (forward edges go in flow direction)
// - Aspect ratio control via column-limited wrapping at sqrt(N)
// - Local improvement pass (swap/move) to reduce total edge length
func gridPlacement(g *d2graph.Graph, opts *ConfigurableOpts) *gridInfo {
	objects := g.Root.ChildrenArray
	if len(objects) == 0 {
		return &gridInfo{}
	}
	n := len(objects)
	channel := 80.0

	// Build adjacency list, degree map, and directed edge info.
	objIndex := make(map[*d2graph.Object]int, n)
	for i, obj := range objects {
		objIndex[obj] = i
	}

	adj := make([][]int, n)
	// outgoing[i] tracks which neighbors i has outgoing edges to (i → nb).
	outgoing := make([]map[int]bool, n)
	for i := range outgoing {
		outgoing[i] = make(map[int]bool)
	}
	for _, e := range g.Edges {
		si, sok := objIndex[e.Src]
		di, dok := objIndex[e.Dst]
		if sok && dok && si != di {
			adj[si] = append(adj[si], di)
			adj[di] = append(adj[di], si)
			outgoing[si][di] = true
		}
	}

	// Deduplicate adjacency lists.
	for i := range adj {
		seen := make(map[int]bool)
		deduped := adj[i][:0]
		for _, nb := range adj[i] {
			if !seen[nb] {
				seen[nb] = true
				deduped = append(deduped, nb)
			}
		}
		adj[i] = deduped
	}

	degree := make([]int, n)
	for i := range adj {
		degree[i] = len(adj[i])
	}

	// Find the max-degree node as the BFS start (center of the layout).
	start := 0
	for i := 1; i < n; i++ {
		if degree[i] > degree[start] {
			start = i
		}
	}

	// [FORK] Determine BFS expansion direction from graph's direction attribute.
	dirs := bfsDirs(g)

	// [FORK] Aspect ratio control: limit columns to ~sqrt(N).
	maxCols := int(math.Ceil(math.Sqrt(float64(n))))
	if maxCols < 2 {
		maxCols = 2
	}

	// BFS to assign grid cells.
	occupied := make(map[gridCell]bool)
	placement := make(map[int]gridCell, n)

	placement[start] = gridCell{0, 0}
	occupied[gridCell{0, 0}] = true

	queue := []int{start}
	visited := make([]bool, n)
	visited[start] = true

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		curCell := placement[cur]

		// Collect unvisited neighbors.
		neighbors := make([]int, 0, len(adj[cur]))
		for _, nb := range adj[cur] {
			if !visited[nb] {
				neighbors = append(neighbors, nb)
			}
		}

		// [FORK] Sort neighbors: higher degree first, with forward-edge bias.
		// Forward neighbors (cur → nb) get priority in the primary BFS direction.
		sort.Slice(neighbors, func(i, j int) bool {
			di, dj := degree[neighbors[i]], degree[neighbors[j]]
			if di != dj {
				return di > dj
			}
			// Tiebreak: forward edges before backward edges.
			fi := outgoing[cur][neighbors[i]]
			fj := outgoing[cur][neighbors[j]]
			if fi != fj {
				return fi
			}
			return neighbors[i] < neighbors[j]
		})

		for _, nb := range neighbors {
			if visited[nb] {
				continue
			}
			visited[nb] = true

			// [FORK] Edge-direction-aware placement: prefer forward direction for outgoing edges.
			preferredDirs := dirs
			if outgoing[cur][nb] {
				// cur → nb: prefer forward direction (first in dirs).
				preferredDirs = dirs
			} else {
				// nb → cur: prefer backward direction (opposite of first in dirs).
				preferredDirs = reverseDirs(dirs)
			}

			cell := findBestCell(curCell, occupied, preferredDirs, maxCols)
			placement[nb] = cell
			occupied[cell] = true
			queue = append(queue, nb)
		}
	}

	// Handle disconnected nodes.
	for i := 0; i < n; i++ {
		if !visited[i] {
			cell := findFirstFree(occupied)
			placement[i] = cell
			occupied[cell] = true
			visited[i] = true
		}
	}

	// [FORK] Local improvement pass: swap/move to reduce total edge length.
	localImprove(placement, occupied, adj, objects, 5)

	// Normalize: shift so minimum row/col is 0.
	minRow, minCol := math.MaxInt32, math.MaxInt32
	maxRow, maxCol := math.MinInt32, math.MinInt32
	for _, cell := range placement {
		if cell.row < minRow {
			minRow = cell.row
		}
		if cell.col < minCol {
			minCol = cell.col
		}
		if cell.row > maxRow {
			maxRow = cell.row
		}
		if cell.col > maxCol {
			maxCol = cell.col
		}
	}

	// [FORK] Variable cell sizes: per-column width, per-row height.
	colWidth := make(map[int]float64)
	rowHeight := make(map[int]float64)
	for i, obj := range objects {
		cell := placement[i]
		c := cell.col - minCol
		r := cell.row - minRow
		w := obj.Width + channel
		h := obj.Height + channel
		if w > colWidth[c] {
			colWidth[c] = w
		}
		if h > rowHeight[r] {
			rowHeight[r] = h
		}
	}

	// Ensure minimum cell dimensions for empty columns/rows.
	minCellDim := channel
	numCols := maxCol - minCol + 1
	numRows := maxRow - minRow + 1
	for c := 0; c < numCols; c++ {
		if colWidth[c] < minCellDim {
			colWidth[c] = minCellDim
		}
	}
	for r := 0; r < numRows; r++ {
		if rowHeight[r] < minCellDim {
			rowHeight[r] = minCellDim
		}
	}

	// Prefix sums for pixel positions.
	colX := make(map[int]float64)
	rowY := make(map[int]float64)
	x := 0.0
	for c := 0; c < numCols; c++ {
		colX[c] = x
		x += colWidth[c]
	}
	y := 0.0
	for r := 0; r < numRows; r++ {
		rowY[r] = y
		y += rowHeight[r]
	}

	// Build occupied map with node indices.
	occupiedIdx := make(map[gridCell]int)
	for i := range objects {
		cell := placement[i]
		normalized := gridCell{cell.row - minRow, cell.col - minCol}
		occupiedIdx[normalized] = i
	}

	// Normalize placement and assign pixel positions.
	normalizedPlacement := make(map[int]gridCell, n)
	for i, obj := range objects {
		cell := placement[i]
		r := cell.row - minRow
		c := cell.col - minCol
		normalizedPlacement[i] = gridCell{r, c}

		cx := colX[c] + colWidth[c]/2
		cy := rowY[r] + rowHeight[r]/2
		obj.TopLeft = geo.NewPoint(cx-obj.Width/2, cy-obj.Height/2)
	}

	return &gridInfo{
		placement: normalizedPlacement,
		occupied:  occupiedIdx,
		colWidth:  colWidth,
		rowHeight: rowHeight,
		colX:      colX,
		rowY:      rowY,
		objects:   objects,
		channel:   channel,
	}
}

// bfsDirs returns BFS expansion directions based on graph's direction attribute.
func bfsDirs(g *d2graph.Graph) []gridCell {
	dir := ""
	if g.Root != nil && g.Root.Attributes.Direction.Value != "" {
		dir = g.Root.Attributes.Direction.Value
	}
	switch dir {
	case "down":
		return []gridCell{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} // down, right, up, left
	case "up":
		return []gridCell{{-1, 0}, {0, 1}, {1, 0}, {0, -1}} // up, right, down, left
	case "left":
		return []gridCell{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} // left, down, right, up
	default: // "right" or unset
		return []gridCell{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} // right, down, left, up
	}
}

// reverseDirs returns dirs with the primary direction reversed (for backward edges).
func reverseDirs(dirs []gridCell) []gridCell {
	if len(dirs) < 1 {
		return dirs
	}
	// Put the opposite of the primary direction first.
	rev := make([]gridCell, len(dirs))
	rev[0] = gridCell{-dirs[0].row, -dirs[0].col}
	copy(rev[1:], dirs[1:])
	return rev
}

// findBestCell finds the best unoccupied cell near `center`.
// It tries direct neighbors first (in priority order), then spirals outward.
// maxCols limits horizontal expansion for aspect ratio control — cells beyond
// column ±maxCols from the origin are deprioritized (tried only as fallback).
func findBestCell(center gridCell, occupied map[gridCell]bool, dirs []gridCell, maxCols int) gridCell {
	// Try immediate neighbors first, preferring within column bounds.
	for _, d := range dirs {
		candidate := gridCell{center.row + d.row, center.col + d.col}
		if !occupied[candidate] && abs(candidate.col) < maxCols {
			return candidate
		}
	}
	// Retry without column constraint for immediate neighbors.
	for _, d := range dirs {
		candidate := gridCell{center.row + d.row, center.col + d.col}
		if !occupied[candidate] {
			return candidate
		}
	}

	// Spiral outward: first pass within maxCols, then without constraint.
	for radius := 2; radius <= 20; radius++ {
		for dr := -radius; dr <= radius; dr++ {
			for dc := -radius; dc <= radius; dc++ {
				if abs(dr) != radius && abs(dc) != radius {
					continue
				}
				candidate := gridCell{center.row + dr, center.col + dc}
				if !occupied[candidate] && abs(candidate.col) < maxCols {
					return candidate
				}
			}
		}
	}
	// Fallback: allow any column.
	for radius := 2; radius <= 20; radius++ {
		for dr := -radius; dr <= radius; dr++ {
			for dc := -radius; dc <= radius; dc++ {
				if abs(dr) != radius && abs(dc) != radius {
					continue
				}
				candidate := gridCell{center.row + dr, center.col + dc}
				if !occupied[candidate] {
					return candidate
				}
			}
		}
	}

	return gridCell{center.row, center.col + 100}
}

// findFirstFree finds the first unoccupied cell scanning from (0,0) outward.
func findFirstFree(occupied map[gridCell]bool) gridCell {
	for radius := 0; radius <= 50; radius++ {
		for r := -radius; r <= radius; r++ {
			for c := -radius; c <= radius; c++ {
				cell := gridCell{r, c}
				if !occupied[cell] {
					return cell
				}
			}
		}
	}
	return gridCell{0, len(occupied)}
}

// localImprove performs swap/move optimization to reduce a cost that combines
// Manhattan edge length + crossing penalty (edges passing through occupied cells).
// Two layouts can have identical Manhattan distance but very different crossing counts.
// The crossing penalty ensures the optimizer prefers layouts where edges don't need Z-routes.
func localImprove(placement map[int]gridCell, occupied map[gridCell]bool, adj [][]int, objects []*d2graph.Object, maxIters int) {
	n := len(objects)
	if n <= 2 {
		return
	}

	const crossingPenalty = 4 // each occupied cell between src and dst adds this to cost

	// costForEdge returns Manhattan distance + penalty for cells between src and dst
	// that are occupied by other nodes (would require Z-routing).
	costForEdge := func(ci, cj gridCell) int {
		dist := abs(ci.row-cj.row) + abs(ci.col-cj.col)
		if dist <= 1 {
			return dist // adjacent cells never cross anything
		}
		penalty := 0
		if ci.row == cj.row {
			// Same row: count occupied cells between them.
			minC, maxC := ci.col, cj.col
			if minC > maxC {
				minC, maxC = maxC, minC
			}
			for c := minC + 1; c < maxC; c++ {
				if occupied[gridCell{ci.row, c}] {
					penalty += crossingPenalty
				}
			}
		} else if ci.col == cj.col {
			// Same column: count occupied cells between them.
			minR, maxR := ci.row, cj.row
			if minR > maxR {
				minR, maxR = maxR, minR
			}
			for r := minR + 1; r < maxR; r++ {
				if occupied[gridCell{r, ci.col}] {
					penalty += crossingPenalty
				}
			}
		} else {
			// Diagonal: check if any cell along the dominant axis blocks an L-route.
			// An L-route from (r1,c1) to (r2,c2) bends at (r1,c2) or (r2,c1).
			// Check both corners — if either is free of blocking cells, no penalty.
			corner1blocked := false
			corner2blocked := false

			// Corner 1: (ci.row, cj.col) — horizontal first, then vertical.
			minC, maxC := ci.col, cj.col
			if minC > maxC {
				minC, maxC = maxC, minC
			}
			for c := minC + 1; c < maxC; c++ {
				if occupied[gridCell{ci.row, c}] {
					corner1blocked = true
					break
				}
			}
			if !corner1blocked {
				minR, maxR := ci.row, cj.row
				if minR > maxR {
					minR, maxR = maxR, minR
				}
				for r := minR + 1; r < maxR; r++ {
					if occupied[gridCell{r, cj.col}] {
						corner1blocked = true
						break
					}
				}
			}

			// Corner 2: (cj.row, ci.col) — vertical first, then horizontal.
			minR, maxR := ci.row, cj.row
			if minR > maxR {
				minR, maxR = maxR, minR
			}
			for r := minR + 1; r < maxR; r++ {
				if occupied[gridCell{r, ci.col}] {
					corner2blocked = true
					break
				}
			}
			if !corner2blocked {
				minC, maxC = ci.col, cj.col
				if minC > maxC {
					minC, maxC = maxC, minC
				}
				for c := minC + 1; c < maxC; c++ {
					if occupied[gridCell{cj.row, c}] {
						corner2blocked = true
						break
					}
				}
			}

			// Penalty only if BOTH L-route corners are blocked (requires Z-route).
			if corner1blocked && corner2blocked {
				penalty += crossingPenalty
			}
		}
		return dist + penalty
	}

	totalCost := func() int {
		sum := 0
		for i := 0; i < n; i++ {
			for _, nb := range adj[i] {
				if nb > i { // count each edge once
					sum += costForEdge(placement[i], placement[nb])
				}
			}
		}
		return sum
	}

	dirs := []gridCell{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}

	for iter := 0; iter < maxIters; iter++ {
		improved := false
		baseline := totalCost()

		// Try moving each node to an adjacent free cell.
		for i := 0; i < n; i++ {
			origCell := placement[i]
			for _, d := range dirs {
				candidate := gridCell{origCell.row + d.row, origCell.col + d.col}
				if occupied[candidate] {
					continue
				}
				// Temporarily move.
				delete(occupied, origCell)
				occupied[candidate] = true
				placement[i] = candidate

				newCost := totalCost()
				if newCost < baseline {
					baseline = newCost
					improved = true
					break // accept first improvement for this node
				}

				// Revert.
				delete(occupied, candidate)
				occupied[origCell] = true
				placement[i] = origCell
			}
		}

		// Try swapping pairs of nodes.
		for i := 0; i < n && i < 30; i++ {
			for j := i + 1; j < n && j < 30; j++ {
				ci, cj := placement[i], placement[j]
				if ci == cj {
					continue
				}
				// Swap.
				placement[i], placement[j] = cj, ci

				newCost := totalCost()
				if newCost < baseline {
					baseline = newCost
					improved = true
				} else {
					// Revert.
					placement[i], placement[j] = ci, cj
				}
			}
		}

		if !improved {
			break
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
