// [FORK] Orthogonal grid edge router based on:
// Hegemann & Wolff, "A Simple Pipeline for Orthogonal Graph Drawing", GD 2023.
// arXiv:2309.01671. Reference impl: github.com/WueGD/wueortho (Scala 3).

package d2gridrouter

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

// Direction represents the side of a box.
type Direction int

const (
	DirTop    Direction = iota
	DirRight
	DirBottom
	DirLeft
)

// Orientation of a segment.
type Orientation int

const (
	Horizontal Orientation = iota
	Vertical
)

// Rect represents an axis-aligned rectangle.
type Rect struct {
	X, Y, W, H float64 // top-left corner + dimensions
}

func (r Rect) Left() float64   { return r.X }
func (r Rect) Right() float64  { return r.X + r.W }
func (r Rect) Top() float64    { return r.Y }
func (r Rect) Bottom() float64 { return r.Y + r.H }
func (r Rect) CenterX() float64 { return r.X + r.W/2 }
func (r Rect) CenterY() float64 { return r.Y + r.H/2 }

// Port is an edge endpoint on a box boundary.
type Port struct {
	NodeIdx  int       // index into the node list
	EdgeIdx  int       // index into the edge list
	Side     Direction // which side of the box
	Pos      geo.Point // actual position on the box boundary
	IsSrc    bool      // true if this is the source port of the edge
}

// Channel is a maximal empty rectangle between two adjacent boxes (or box and boundary).
// It provides a routing corridor.
type Channel struct {
	Rect        Rect        // the channel rectangle
	Orientation Orientation // Horizontal channel has vertical representative, and vice versa
}

// Segment is a horizontal or vertical line segment used as a channel representative
// or as part of an edge route.
type Segment struct {
	Start       geo.Point
	End         geo.Point
	Orientation Orientation
}

// RoutingGraphNode is a vertex in the routing graph (partial grid).
type RoutingGraphNode struct {
	ID  int
	Pos geo.Point
}

// RoutingGraphEdge connects two adjacent nodes along a representative.
type RoutingGraphEdge struct {
	From, To    int     // node IDs
	Weight      float64 // geometric length
	Orientation Orientation
}

// RoutingGraph is the partial grid used for edge routing.
type RoutingGraph struct {
	Nodes []RoutingGraphNode
	Adj   map[int][]RoutingGraphEdge // adjacency list
}

// EdgeRoute is the result of routing a single edge.
type EdgeRoute struct {
	EdgeIdx int         // index into the original edge list
	Points  []*geo.Point // the orthogonal polyline
}

// PortAssignment holds port assignments for all edges.
type PortAssignment struct {
	SrcPorts []Port // one per edge
	DstPorts []Port // one per edge
}

// DijkstraState is the state tracked during modified Dijkstra.
// Lexicographic ordering: (length, bends).
type DijkstraState struct {
	NodeID    int
	Length    float64
	Bends     int
	Direction Orientation // direction of entry into this node
	Parent    int         // previous node in the path (-1 for start)
}

// Less returns true if a is strictly better than b (lexicographic: length, then bends).
// Uses epsilon comparison for floating-point length to avoid instability.
func (a DijkstraState) Less(b DijkstraState) bool {
	const eps = 1e-9
	if math.Abs(a.Length-b.Length) > eps {
		return a.Length < b.Length
	}
	return a.Bends < b.Bends
}
