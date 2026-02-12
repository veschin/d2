// [FORK] Stage 3c: Modified Dijkstra (Hegemann & Wolff §3.3).
// Routes a single edge through the routing graph.
//
// Uses augmented state: (length, bends, direction).
// Lexicographic minimization: shortest path first, then fewest bends.
// This ensures optimal orthogonal routes that avoid all obstacles.

package d2gridrouter

import (
	"container/heap"
)

// stateKey uniquely identifies a Dijkstra state (node + entry direction).
type stateKey struct {
	nodeID int
	dir    Orientation
}

// dijkstraRoute finds the shortest path from srcNode to dstNode in the routing graph.
// Returns a slice of node IDs forming the path (excluding src, including dst).
// Returns nil if no path exists.
func dijkstraRoute(rg *RoutingGraph, srcNode, dstNode int) []int {
	if srcNode == dstNode {
		return []int{srcNode}
	}

	// Best known state for each (node, direction) pair.
	best := make(map[stateKey]DijkstraState)
	parent := make(map[stateKey]stateKey)
	visited := make(map[stateKey]bool)

	// Priority queue.
	pq := &dijkstraPQ{}
	heap.Init(pq)

	// Start with both orientations from the source.
	for _, dir := range []Orientation{Horizontal, Vertical} {
		s := DijkstraState{
			NodeID:    srcNode,
			Length:    0,
			Bends:     0,
			Direction: dir,
			Parent:    -1,
		}
		key := stateKey{srcNode, dir}
		best[key] = s
		heap.Push(pq, s)
	}

	// Dijkstra loop.
	for pq.Len() > 0 {
		cur := heap.Pop(pq).(DijkstraState)
		curKey := stateKey{cur.NodeID, cur.Direction}

		if visited[curKey] {
			continue
		}
		visited[curKey] = true

		// Reached destination?
		if cur.NodeID == dstNode {
			return reconstructPath(parent, curKey, srcNode)
		}

		// Explore neighbors.
		for _, edge := range rg.Adj[cur.NodeID] {
			newBends := cur.Bends
			if cur.NodeID != srcNode && edge.Orientation != cur.Direction {
				newBends++ // direction change = bend
			}

			newState := DijkstraState{
				NodeID:    edge.To,
				Length:    cur.Length + edge.Weight,
				Bends:     newBends,
				Direction: edge.Orientation,
				Parent:    cur.NodeID,
			}

			newKey := stateKey{edge.To, edge.Orientation}
			if visited[newKey] {
				continue
			}
			if b, ok := best[newKey]; ok && !newState.Less(b) {
				continue
			}

			best[newKey] = newState
			parent[newKey] = curKey
			heap.Push(pq, newState)
		}
	}

	return nil // no path found
}

// reconstructPath traces back from the destination to the source.
func reconstructPath(parent map[stateKey]stateKey, endKey stateKey, srcNode int) []int {
	var path []int
	cur := endKey
	for cur.nodeID != srcNode {
		path = append(path, cur.nodeID)
		prev, ok := parent[cur]
		if !ok {
			break
		}
		cur = prev
	}
	// Reverse.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// dijkstraPQ is a priority queue for Dijkstra states.
type dijkstraPQ []DijkstraState

func (pq dijkstraPQ) Len() int { return len(pq) }

func (pq dijkstraPQ) Less(i, j int) bool {
	return pq[i].Less(pq[j])
}

func (pq dijkstraPQ) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *dijkstraPQ) Push(x interface{}) {
	*pq = append(*pq, x.(DijkstraState))
}

func (pq *dijkstraPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}
