// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package token

// avgTokensPerFile is the median token count per file in a real GitHub org.
// Derived by sampling 50 representative files from kubernetes/kubernetes and
// averaging their cl100k_base token counts.
const avgTokensPerFile = 1847

// filesNeededNaive is the estimated number of files an AI must read (without DocScout)
// to answer each canonical question from questions.json (index-matched).
var filesNeededNaive = [12]int{
	15, // "Which services depend on billing-service?" — read go.mod of ~15 repos
	8,  // "Who owns the checkout service?" — read CODEOWNERS of ~8 repos
	20, // "What would break if database goes down?" — traverse all dependent repos
	25, // "List all services that expose a gRPC endpoint" — read all .proto files
	20, // "Which repos have no CODEOWNERS?" — check CODEOWNERS presence in all repos
	12, // "What Go services depend on billing-service directly?" — all go.mod files
	10, // "Which teams own more than one service?" — all CODEOWNERS
	5,  // "What events does payment-worker publish?" — asyncapi files
	18, // "Find the shortest dependency path..." — traverse via repeated file reads
	20, // "Which services have no OpenAPI spec?" — check all repos
	10, // "What is the Go version of billing-service?" — targeted go.mod read + check
	15, // "List all services that depend on kafka-client" — all pom.xml + go.mod
}

// docScoutTypicalTokens is the estimated token count of a DocScout tool response
// for each canonical question (index-matched).
var docScoutTypicalTokens = [12]int{
	320,  // traverse_graph result: 1-2 entity JSON objects
	180,  // open_nodes result: 1 entity with owner relation
	450,  // traverse_graph depth=2 result
	280,  // list_entities filtered by type=grpc-service
	210,  // list_repos with no CODEOWNERS flag
	300,  // search_nodes + traverse_graph
	240,  // list_entities type=team + traverse_graph
	190,  // get_integration_map for payment-worker
	380,  // find_path result
	220,  // list_repos filtered
	150,  // open_nodes for billing-service, read go_version obs
	290,  // search_nodes kafka-client + traverse_graph incoming
}

// EstimateNaiveTokens returns the estimated token cost for a naive AI (no DocScout)
// to answer question at index i. Returns 0 for out-of-range indices.
func EstimateNaiveTokens(i int) int {
	if i < 0 || i >= len(filesNeededNaive) {
		return 0
	}
	return filesNeededNaive[i] * avgTokensPerFile
}

// EstimateDocScoutTokens returns the estimated token cost when using DocScout tools
// to answer question at index i. Returns 0 for out-of-range indices.
func EstimateDocScoutTokens(i int) int {
	if i < 0 || i >= len(docScoutTypicalTokens) {
		return 0
	}
	return docScoutTypicalTokens[i]
}

// SavingsPct returns the percentage of tokens saved by using DocScout vs naive.
func SavingsPct(docscout, naive int) float64 {
	if naive == 0 {
		return 0
	}
	return float64(naive-docscout) / float64(naive) * 100
}

// QuestionEstimate holds theoretical token estimates for one canonical question.
type QuestionEstimate struct {
	Index        int
	DocScoutToks int
	NaiveToks    int
	SavingsPct   float64
}

// AllEstimates returns theoretical estimates for all 12 canonical questions.
func AllEstimates() []QuestionEstimate {
	out := make([]QuestionEstimate, 12)
	for i := range out {
		ds := EstimateDocScoutTokens(i)
		nv := EstimateNaiveTokens(i)
		out[i] = QuestionEstimate{
			Index:        i,
			DocScoutToks: ds,
			NaiveToks:    nv,
			SavingsPct:   SavingsPct(ds, nv),
		}
	}
	return out
}
