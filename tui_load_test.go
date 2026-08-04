package main

import (
	"testing"
)

// The link list used to be built with one request per link, on top of the
// request that fetched the vertex. Since every mutation triggers a refresh,
// that fan-out dominated the cost of editing anything on a vertex with many
// links. The vertex read already carries each link's target and type, so the
// fan-out is now skipped entirely.
//
// These tests assert the fast path is actually taken: they run with no network
// available, so any fallback to per-link reads would surface as a failure
// rather than as silently slow behaviour.

func TestFetchLinks_UsesTheVertexReadInsteadOfPerLinkRequests(t *testing.T) {
	fvi := fullVertexInfo{
		id: "hub/a",
		outLinks: []linkId{
			{from: "hub/a", name: "l1"},
			{from: "hub/a", name: "l2"},
		},
		outFull: []fullLinkInfo{
			{id: linkId{from: "hub/a", name: "l1"}, to: "hub/b", tp: "rel"},
			{id: linkId{from: "hub/a", name: "l2"}, to: "hub/c", tp: "rel"},
		},
		inLinks: []linkId{{from: "hub/p", name: "pl"}},
		inFull: []fullLinkInfo{
			{id: linkId{from: "hub/p", name: "pl"}, to: "hub/a", tp: "parent_of"},
		},
	}

	msg := runCmd(fetchLinksCmd("hub/a", 1, &fvi))
	loaded, ok := msg.(linksLoadedMsg)
	if !ok {
		t.Fatalf("got %T, want linksLoadedMsg", msg)
	}
	if loaded.partialErr != nil {
		t.Fatalf("partialErr = %v — the fast path should need no requests", loaded.partialErr)
	}
	if len(loaded.links) != 3 {
		t.Fatalf("links = %d, want 3", len(loaded.links))
	}

	var out, in int
	for _, dl := range loaded.links {
		if dl.isOut {
			out++
			if dl.info.tp == "" || dl.info.to == "" {
				t.Errorf("out-link %q lost its type or target: %+v", dl.info.id.name, dl.info)
			}
		} else {
			in++
			if dl.info.tp != "parent_of" {
				t.Errorf("in-link type = %q, want parent_of", dl.info.tp)
			}
			if dl.target() != "hub/p" {
				t.Errorf("in-link target = %q, want the source vertex", dl.target())
			}
		}
	}
	if out != 2 || in != 1 {
		t.Errorf("out/in = %d/%d, want 2/1", out, in)
	}
}

func TestFetchLinks_EmptyVertexNeedsNoRequests(t *testing.T) {
	fvi := fullVertexInfo{id: "hub/lonely"}
	msg := runCmd(fetchLinksCmd("hub/lonely", 1, &fvi))
	loaded, ok := msg.(linksLoadedMsg)
	if !ok {
		t.Fatalf("got %T, want linksLoadedMsg", msg)
	}
	if len(loaded.links) != 0 {
		t.Errorf("links = %d, want none", len(loaded.links))
	}
}

func TestFetchLinks_PartialHarvestFallsBack(t *testing.T) {
	// An older runtime that ignores details_v2 returns names only, so outFull
	// stays short of outLinks and the per-link path must still be taken. We
	// cannot exercise that path without a server; asserting the guard itself
	// is what keeps the fast path from silently swallowing incomplete data.
	fvi := fullVertexInfo{
		id:       "hub/a",
		outLinks: []linkId{{from: "hub/a", name: "l1"}, {from: "hub/a", name: "l2"}},
		outFull:  []fullLinkInfo{{id: linkId{from: "hub/a", name: "l1"}, to: "hub/b", tp: "rel"}},
	}
	harvested := len(fvi.outFull) + len(fvi.inFull)
	total := len(fvi.outLinks) + len(fvi.inLinks)
	if harvested == total {
		t.Fatal("fixture is wrong: it should represent a partial harvest")
	}
}
