// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package memory

// TraverseNode is a node reached during graph traversal, enriched with
// distance from the start entity and the path of entity names leading to it.
type TraverseNode struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
	Distance     int      `json:"distance"`
	Path         []string `json:"path"` // entity names from start (exclusive) to this node (inclusive)
}

// TraverseEdge is a directed edge discovered during graph traversal.
type TraverseEdge struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
	Confidence   string `json:"confidence,omitempty"`
}

// traverseGraph performs BFS from entity up to maxDepth hops.
// direction must be "outgoing", "incoming", or "both".
// relationType filters edges by type; empty string matches all types.
// The start entity itself is not included in the results.
func (s store) traverseGraph(entity, relationType, direction string, maxDepth int) ([]TraverseNode, []TraverseEdge, error) {
	type bfsEntry struct {
		name string
		path []string
	}

	visited := map[string]bool{entity: true}
	frontier := []bfsEntry{{name: entity, path: []string{entity}}}
	var results []TraverseNode
	var edges []TraverseEdge

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		frontierNames := make([]string, len(frontier))
		for i, e := range frontier {
			frontierNames[i] = e.name
		}

		// pathOf maps a frontier entity name → its path slice, so we can
		// extend it for each discovered neighbour.
		pathOf := make(map[string][]string, len(frontier))
		for _, e := range frontier {
			pathOf[e.name] = e.path
		}

		edgeRows, err := s.queryNeighbours(frontierNames, relationType, direction)
		if err != nil {
			return nil, nil, err
		}

		var nextFrontier []bfsEntry
		for _, row := range edgeRows {
			parentPath := pathOf[row.FromNode]
			target := row.ToNode
			edges = append(edges, TraverseEdge{
				From:         row.FromNode,
				To:           target,
				RelationType: row.RelationType,
				Confidence:   row.Confidence,
			})
			if visited[target] {
				continue
			}
			visited[target] = true
			nodePath := make([]string, len(parentPath), len(parentPath)+1)
			copy(nodePath, parentPath)
			nodePath = append(nodePath, target)

			results = append(results, TraverseNode{
				Name:     target,
				Distance: depth,
				Path:     nodePath[1:], // strip the start entity; path = nodes traversed to reach target
			})
			nextFrontier = append(nextFrontier, bfsEntry{name: target, path: nodePath})
		}
		frontier = nextFrontier
	}

	if len(results) == 0 {
		return results, edges, nil
	}

	// Batch-load entity types and observations for all discovered nodes.
	names := make([]string, len(results))
	for i, n := range results {
		names[i] = n.Name
	}

	var dbEntities []dbEntity
	if err := s.db.Where("name IN ?", names).Find(&dbEntities).Error; err != nil {
		return nil, nil, err
	}
	typeOf := make(map[string]string, len(dbEntities))
	for _, e := range dbEntities {
		typeOf[e.Name] = e.EntityType
	}

	var dbObs []dbObservation
	if err := s.db.Where("entity_name IN ?", names).Find(&dbObs).Error; err != nil {
		return nil, nil, err
	}
	obsOf := make(map[string][]string)
	for _, o := range dbObs {
		obsOf[o.EntityName] = append(obsOf[o.EntityName], o.Content)
	}

	for i := range results {
		results[i].EntityType = typeOf[results[i].Name]
		results[i].Observations = obsOf[results[i].Name]
	}

	return results, edges, nil
}

// queryNeighbours returns edge rows for the given frontier and direction.
// For "both", it merges outgoing and incoming results.
func (s store) queryNeighbours(frontier []string, relationType, direction string) ([]edgeRow, error) {
	var result []edgeRow

	if direction == "outgoing" || direction == "both" {
		rows, err := s.edgeQuery(frontier, relationType, "outgoing")
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
	}

	if direction == "incoming" || direction == "both" {
		rows, err := s.edgeQuery(frontier, relationType, "incoming")
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
	}

	return result, nil
}

type edgeRow struct {
	FromNode     string `gorm:"column:from_node"`
	ToNode       string `gorm:"column:to_node"`
	RelationType string `gorm:"column:relation_type"`
	Confidence   string `gorm:"column:confidence"`
}

// edgeQuery fetches one hop of edges for the given frontier nodes.
// For outgoing: frontier nodes are from_node, neighbours are to_node.
// For incoming: frontier nodes are to_node, neighbours are from_node
//
//	(aliased so the result map is always keyed by the frontier node).
func (s store) edgeQuery(frontier []string, relationType, direction string) ([]edgeRow, error) {
	var rows []edgeRow

	db := s.db.Model(&dbRelation{})
	if direction == "outgoing" {
		db = db.Select("from_node, to_node, relation_type, confidence").Where("from_node IN ?", frontier)
	} else {
		// Swap columns so FromNode is always the frontier side.
		db = db.Select("to_node AS from_node, from_node AS to_node, relation_type, confidence").Where("to_node IN ?", frontier)
	}
	if relationType != "" {
		db = db.Where("relation_type = ?", relationType)
	}
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
