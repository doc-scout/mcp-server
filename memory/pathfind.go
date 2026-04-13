// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

// PathEdge is a single directed edge on the path between two entities.
// The edge reflects the actual stored direction regardless of traversal direction.
type PathEdge struct {
	From         string `json:"from"`
	RelationType string `json:"relationType"`
	To           string `json:"to"`
}

// pathEdgeRow is used for scanning raw edge rows that include the relation type.
type pathEdgeRow struct {
	FromNode     string `gorm:"column:from_node"`
	ToNode       string `gorm:"column:to_node"`
	RelationType string `gorm:"column:relation_type"`
}

// pathParentInfo records how a node was reached during BFS.
type pathParentInfo struct {
	parent string
	edge   PathEdge // directed edge (original stored direction) that led here
}

// findPath performs a bidirectional-aware BFS from `from`, following all edges in
// both directions, and returns the shortest path to `to` as an ordered slice of
// directed edges (reflecting the original stored direction).
//
// Returns an empty slice when no path exists within maxDepth hops.
// maxDepth is clamped to [1, 10].
func (s store) findPath(from, to string, maxDepth int) ([]PathEdge, error) {
	if maxDepth < 1 {
		maxDepth = 1
	}
	if maxDepth > 10 {
		maxDepth = 10
	}
	if from == to {
		return []PathEdge{}, nil
	}

	visited := map[string]bool{from: true}
	parent := map[string]pathParentInfo{}
	frontier := []string{from}

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		edges, err := s.allEdgesFor(frontier)
		if err != nil {
			return nil, err
		}

		var nextFrontier []string
		for _, e := range edges {
			// Each stored edge gives us two traversal directions.
			// outgoing: from -> to  (frontier node = from, neighbour = to)
			// incoming: to -> from  (frontier node = to, neighbour = from)
			candidates := []struct {
				frontierNode, neighbour string
				edge                    PathEdge
			}{
				{e.FromNode, e.ToNode, PathEdge{From: e.FromNode, RelationType: e.RelationType, To: e.ToNode}},
				{e.ToNode, e.FromNode, PathEdge{From: e.FromNode, RelationType: e.RelationType, To: e.ToNode}},
			}

			for _, c := range candidates {
				if !visited[c.frontierNode] {
					continue
				}
				if visited[c.neighbour] {
					continue
				}
				visited[c.neighbour] = true
				parent[c.neighbour] = pathParentInfo{parent: c.frontierNode, edge: c.edge}

				if c.neighbour == to {
					return reconstructPath(parent, from, to), nil
				}
				nextFrontier = append(nextFrontier, c.neighbour)
			}
		}
		frontier = nextFrontier
	}

	return []PathEdge{}, nil // no path found
}

// allEdgesFor returns all edges (in either direction) that touch any of the frontier nodes.
func (s store) allEdgesFor(frontier []string) ([]pathEdgeRow, error) {
	var rows []pathEdgeRow
	err := s.db.Model(&dbRelation{}).
		Select("from_node, to_node, relation_type").
		Where("from_node IN ? OR to_node IN ?", frontier, frontier).
		Find(&rows).Error
	return rows, err
}

// reconstructPath walks the parent map backwards from `to` to `from`
// and returns the edge sequence in forward order.
func reconstructPath(parent map[string]pathParentInfo, from, to string) []PathEdge {
	var path []PathEdge
	cur := to
	for cur != from {
		info := parent[cur]
		path = append(path, info.edge)
		cur = info.parent
	}
	// reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
