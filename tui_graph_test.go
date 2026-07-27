package main

import (
	"testing"

	"github.com/foliagecp/easyjson"
)

// ── Canonical ids ─────────────────────────────────────────────────────────────

func TestCanonID_QualifiesBareAndLeavesQualifiedAlone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"srv", "hub/srv"},
		{"hub/srv", "hub/srv"},
		{"leaf/srv", "leaf/srv"},
		{"", ""},
	}
	for _, c := range cases {
		if got := canonID(c.in); got != c.want {
			t.Errorf("canonID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCanonID_IsIdempotent(t *testing.T) {
	// It is applied at several boundaries and they overlap; applying it twice
	// must not produce hub/hub/srv.
	if got := canonID(canonID("srv")); got != "hub/srv" {
		t.Errorf("canonID twice = %q, want hub/srv", got)
	}
}

func TestCanonIDIn_KeepsSomethingInItsOwnDomain(t *testing.T) {
	if got := canonIDIn("x", "leaf"); got != "leaf/x" {
		t.Errorf("canonIDIn(x, leaf) = %q, want leaf/x", got)
	}
}

// ── Classification ────────────────────────────────────────────────────────────

func TestClassifyVertex_Table(t *testing.T) {
	inLink := func(from, name, tp string) displayLink {
		return displayLink{info: makeLinkInfo(from, name, "hub/self", tp), isOut: false}
	}
	outLink := func(name, to, tp string) displayLink {
		return displayLink{info: makeLinkInfo("hub/self", name, to, tp), isOut: true}
	}

	cases := []struct {
		name     string
		id       string
		links    []displayLink
		wantKind vertexKind
		wantType string
	}{
		{
			// The reported bug. root points AT the types root; a type is
			// pointed at BY it. Direction is the entire difference.
			name: "root is structural, not a type",
			id:   "hub/root",
			links: []displayLink{
				outLink("types", "hub/types", ltTypesRoot),
				outLink("objects", "hub/objects", ltObjectsRt),
			},
			wantKind: vkStructural,
		},
		{name: "the types root itself", id: "hub/types",
			links:    []displayLink{outLink("srv", "hub/srv", ltInstanceOf)},
			wantKind: vkStructural},
		{name: "the objects root itself", id: "hub/objects",
			links:    []displayLink{outLink("hub/srv-1", "hub/srv-1", ltInstance)},
			wantKind: vkStructural},
		{name: "a type", id: "hub/srv",
			links:    []displayLink{inLink("hub/types", "srv", ltInstanceOf)},
			wantKind: vkType},
		{
			// A type's schema links are outgoing __type edges too. Without the
			// types-root test running first, a type that declares a types-link
			// would read as an object of its link target.
			name: "a type that declares a types-link is still a type",
			id:   "hub/srv",
			links: []displayLink{
				inLink("hub/types", "srv", ltInstanceOf),
				outLink("rack", "hub/rack", ltInstanceOf),
			},
			wantKind: vkType,
		},
		{
			name: "an object names its type",
			id:   "hub/srv-1",
			links: []displayLink{
				inLink("hub/objects", "hub/srv-1", ltInstance),
				inLink("hub/srv", "hub/srv-1", ltInstance),
				outLink(instanceOfLinkName, "hub/srv", ltInstanceOf),
			},
			wantKind: vkObject, wantType: "srv",
		},
		{name: "half-written object", id: "hub/srv-1",
			links:    []displayLink{inLink("hub/objects", "hub/srv-1", ltInstance)},
			wantKind: vkBrokenObject},
		{name: "plain vertex", id: "hub/x",
			links:    []displayLink{outLink("l", "hub/y", "rel")},
			wantKind: vkPlain},
		{name: "no links at all", id: "hub/x", wantKind: vkPlain},
	}

	for _, c := range cases {
		k, tp := classifyVertex(c.id, c.links)
		if k != c.wantKind {
			t.Errorf("%s: kind = %v, want %v", c.name, k, c.wantKind)
		}
		if tp != c.wantType {
			t.Errorf("%s: type name = %q, want %q", c.name, tp, c.wantType)
		}
	}
}

// ── Link tiers ────────────────────────────────────────────────────────────────

func TestTierOfExistingLink_FollowsOwnershipNotTheScreen(t *testing.T) {
	// The same schema edge, seen from each end. The API addresses a link from
	// its owner, so for an incoming link the owner is the far vertex — reading
	// the kinds off "near/far" instead would send an inbound types-link
	// through the objects API.
	outFromSrv := displayLink{info: makeLinkInfo("hub/srv", "rack", "hub/rack", ltInstanceOf), isOut: true}
	if got := tierOfExistingLink(outFromSrv, vkType, vkType, false); got != tierTypesLink {
		t.Errorf("outgoing schema edge = %v, want tierTypesLink", got)
	}
	inAtRack := displayLink{info: makeLinkInfo("hub/srv", "rack", "hub/rack", ltInstanceOf), isOut: false}
	if got := tierOfExistingLink(inAtRack, vkType, vkType, false); got != tierTypesLink {
		t.Errorf("the same edge seen from the target = %v, want tierTypesLink", got)
	}
}

func TestTierOfExistingLink_DowngradesWhatTheCMDBCannotAddress(t *testing.T) {
	cases := []struct {
		name           string
		dl             displayLink
		near, far      vertexKind
		llMode         bool
		want           linkTier
		wantExplainNot linkTier
	}{
		{
			name: "an ordinary edge between two types is not a schema link",
			dl:   displayLink{info: makeLinkInfo("hub/a", "l", "hub/b", "rel"), isOut: true},
			near: vkType, far: vkType, want: tierRawLink, wantExplainNot: tierTypesLink,
		},
		{
			// Stored under "<fromClaim>#<toClaim>#<rel>". objects.link.update
			// resolves the BASE types-link between the two objects, so
			// addressing this edge by (from,to) would edit a different one.
			name: "a claimed-type object edge is its own tier",
			dl:   displayLink{info: makeLinkInfo("hub/a", "l", "hub/b", "machine#rack#in"), isOut: true},
			near: vkObject, far: vkObject, want: tierSuperLink,
		},
		{
			name: "the sub-type edge",
			dl:   displayLink{info: makeLinkInfo("hub/hw", "child_srv", "hub/srv", ltSubType), isOut: true},
			near: vkType, far: vkType, want: tierSubType,
		},
		{
			name: "low-level mode overrides everything",
			dl:   displayLink{info: makeLinkInfo("hub/a", "l", "hub/b", ltInstanceOf), isOut: true},
			near: vkType, far: vkType, llMode: true, want: tierRawLink,
		},
		{
			name: "a plain endpoint has no high-level meaning",
			dl:   displayLink{info: makeLinkInfo("hub/a", "l", "hub/b", "rel"), isOut: true},
			near: vkType, far: vkPlain, want: tierRawLink,
		},
	}
	for _, c := range cases {
		if got := tierOfExistingLink(c.dl, c.near, c.far, c.llMode); got != c.want {
			t.Errorf("%s: tier = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestParseSuperLinkType(t *testing.T) {
	from, to, rel, ok := parseSuperLinkType("machine#rack#mounted_in")
	if !ok || from != "machine" || to != "rack" || rel != "mounted_in" {
		t.Errorf("got (%q,%q,%q,%v)", from, to, rel, ok)
	}
	if _, _, _, ok := parseSuperLinkType("plain"); ok {
		t.Error("a plain link type must not parse as a claimed-type one")
	}
}

// ── Link details ──────────────────────────────────────────────────────────────

// TestLinkDetail_ComesFromTheServerNotTheModel is the regression test for the
// silent tag loss. The fast vertex read never fills fullLinkInfo.tags, so a
// form seeded from the model showed an empty tag list over a link that had
// tags — and submitting with `replace` then erased them unseen.
func TestLinkDetail_ComesFromTheServerNotTheModel(t *testing.T) {
	body := easyjson.NewJSONObject()
	body.SetByPath("w", easyjson.NewJSON(1))
	read := easyjson.NewJSONObject()
	read.SetByPath("tags", easyjson.JSONFromArray([]string{"a", "b"}))
	read.SetByPath("body", body)

	withOps(t, graphOps{
		linkRead: func(string, string) (easyjson.JSON, error) { return read, nil },
	})

	// What the fast path actually produces: no tags, no body.
	dl := displayLink{info: makeLinkInfo("hub/a", "l", "hub/b", "rel"), isOut: true}
	if dl.info.tags != nil {
		t.Fatal("fixture is wrong — the fast path leaves tags nil")
	}

	msg := runCmd(fetchLinkDetailCmd(keyOf(dl), 0)).(linkDetailMsg)
	if len(msg.detail.tags) != 2 || msg.detail.tags[0] != "a" {
		t.Errorf("tags = %v, want [a b] read from the server", msg.detail.tags)
	}
	if msg.detail.body.GetByPath("w").AsNumericDefault(0) != 1 {
		t.Error("the body must come from the read, not from the empty model field")
	}
}

func TestLinkDetail_DebounceOnlyFiresForTheLinkStillUnderTheCursor(t *testing.T) {
	m := makeModel("hub/a", threeLinks(), nil)
	m, _ = m.peekCursorLink()
	resting := m.linkPeek

	// The debounce for a link the cursor has since left must do nothing.
	stale := linkPeekMsg{key: linkKey{from: "hub/a", name: "somewhere-else"}, gen: m.loadGen}
	if _, cmd := m.applyLinkPeek(stale); cmd != nil {
		t.Error("a debounce for a link the cursor left must not issue a read")
	}
	// And one for a load that has been superseded.
	if _, cmd := m.applyLinkPeek(linkPeekMsg{key: resting, gen: m.loadGen + 1}); cmd != nil {
		t.Error("a debounce from a superseded load must not issue a read")
	}
}

func TestLinkDetail_ReadIsIssuedOnceAndKeptUntilAWrite(t *testing.T) {
	m := makeModel("hub/a", threeLinks(), nil)
	m, _ = m.peekCursorLink()

	m, cmd := m.applyLinkPeek(linkPeekMsg{key: m.linkPeek, gen: m.loadGen})
	if cmd == nil {
		t.Fatal("resting on an unread link should issue exactly one read")
	}
	// A second debounce while the first is in flight must not double-fetch.
	if _, again := m.applyLinkPeek(linkPeekMsg{key: m.linkPeek, gen: m.loadGen}); again != nil {
		t.Error("a read is already in flight — the debounce must not fire twice")
	}

	m = m.applyLinkDetail(linkDetailMsg{key: m.linkPeek, detail: linkDetail{loaded: true, tags: []string{"x"}}})
	if d, ok := m.linkDetails[m.linkPeek]; !ok || !d.loaded {
		t.Fatal("the detail should be cached once it lands")
	}
	if m.forgetLinkDetails().linkDetails[m.linkPeek].loaded {
		t.Error("a write invalidates edge details along with the vertex cache")
	}
}
