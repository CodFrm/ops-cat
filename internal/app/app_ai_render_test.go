package app

import (
	"testing"
)

// TestBuildDisplayMessages_* tests are skipped pending Task 20 rewrite.
// buildDisplayMessages was stubbed in Task 7 (v1 Message fields removed);
// Phase 3 / Task 20 will rewrite it against the v2 Message schema, at which
// point these tests will be rewritten as well.

func TestBuildDisplayMessages_AssistantTurnAggregatesBlocks(t *testing.T) {
	t.Skip("v1 path removed; await Task 20 rewrite of buildDisplayMessages")
}

func TestBuildDisplayMessages_MultiToolTurnPairsByID(t *testing.T) {
	t.Skip("v1 path removed; await Task 20 rewrite of buildDisplayMessages")
}

func TestBuildDisplayMessages_TurnsBoundedByUser(t *testing.T) {
	t.Skip("v1 path removed; await Task 20 rewrite of buildDisplayMessages")
}

func TestBuildDisplayMessages_OrphanToolCallShowsRunning(t *testing.T) {
	t.Skip("v1 path removed; await Task 20 rewrite of buildDisplayMessages")
}

func TestBuildDisplayMessages_DropsNonDisplayKinds(t *testing.T) {
	t.Skip("v1 path removed; await Task 20 rewrite of buildDisplayMessages")
}

func TestBuildDisplayMessages_MentionsAndTokenUsage(t *testing.T) {
	t.Skip("v1 path removed; await Task 20 rewrite of buildDisplayMessages")
}
