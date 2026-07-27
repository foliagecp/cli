package main

import "strings"

// tui_graph.go answers questions about the GRAPH, as opposed to questions about
// the interface: what is this id in canonical form, what kind of thing is this
// vertex, and which API tier owns a given edge.
//
// It is one file on purpose. The previous arrangement had the answer to "is
// this a type?" living in the view layer as a display heuristic, which the CRUD
// flows then started consulting as a decision procedure. That is exactly how
// `hub/root` came to be offered as a type to create objects under: the heuristic
// asked "does this vertex touch hub/types at all?", which is a fine question for
// deciding whether to print a badge and a terrible one for deciding whether to
// call the CMDB API.

// ── Identity ──────────────────────────────────────────────────────────────────

// The server always speaks fully-qualified ids: an out-link's `to`, an in-link's
// `from`, and every id in a JPGQL result carry a domain. The user, the gwalk
// cursor file and every form field speak bare ones. Mixing the two forms in one
// map is what made a freshly created object invisible: the object was created
// from `hub/srv`, the cache held the type under `"hub/srv"`, and the
// invalidation named it `"srv"` — so walking back to the type replayed a link
// list captured before the object existed.
//
// The rule is therefore: EVERY id that enters the model is canonical. Bare ids
// exist only in what the user types and in what is printed for them to read.

// canonID returns id in the form the model stores it: domain-qualified.
// Ids that already carry a domain are returned unchanged, so it is safe to
// apply to server-supplied ids and safe to apply twice.
func canonID(id string) string { return canonIDIn(id, NatsHubDomain) }

// canonIDIn qualifies a bare id into the given domain. Use it when creating
// something next to a vertex the user is standing on, so a leaf-domain vertex
// does not silently acquire a hub-domain sibling.
func canonIDIn(id, domain string) string {
	if id == "" {
		return ""
	}
	if strings.Contains(id, "/") {
		return id
	}
	if domain == "" {
		domain = NatsHubDomain
	}
	return domain + "/" + id
}

// stripDomain is a DISPLAY function. It must not be used to build an id that
// will be stored, compared or sent — use canonID for that.
func stripDomain(id string) string {
	if i := strings.Index(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func domainOf(id string) string {
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	return NatsHubDomain
}

// hubID names one of the graph's structural roots.
func hubID(name string) string { return NatsHubDomain + "/" + name }

// ── Link types the server owns ────────────────────────────────────────────────

// These mirror the constants in the SDK's embedded/graph/crud package. They are
// restated rather than imported because the pinned module does not export them,
// and because the TUI needs to recognise them in data it merely read.
const (
	ltInstanceOf = "__type"   // types → type, and object → its type
	ltInstance   = "__object" // objects → object, and type → its instances
	ltTypesRoot  = "__types"  // root → the types vertex
	ltObjectsRt  = "__objects"
	ltSubType    = "__sub" // type → sub-type

	// instanceOfLinkName is the name the server gives an object's instance-of
	// edge (createObjectInline writes it literally). It is what distinguishes
	// that edge from a types-link, which shares the same link TYPE but is named
	// after its target.
	instanceOfLinkName = "type"
)

// ── Vertex classification ─────────────────────────────────────────────────────

type vertexKind int

const (
	vkPlain        vertexKind = iota // a bare graph vertex — only the low-level API applies
	vkType                           // a CMDB type: the types root links IN to it
	vkObject                         // a CMDB object: has an instance-of edge to its type
	vkStructural                     // root / types / objects — the CMDB topology itself
	vkBrokenObject                   // linked from a type, but its instance-of edge is missing
)

// classifyVertex decides what a vertex is from its id and its links, and for an
// object also returns the name of its type.
//
// It follows the SERVER's rule rather than a cheaper approximation. ReadType
// walks `links.in` looking for the types root; so does this. Direction is the
// whole discriminator: a type is a vertex the types root points AT, while
// `root` is the vertex that points at the types root. Ignoring direction —
// which is what the old heuristic did — makes those two indistinguishable, and
// `root` then classifies as a type because it happens to be adjacent to
// `hub/types`.
//
// Link type matters too, and for the same reason. `__types` (plural) is root's
// structural edge; `__type` (singular) is the membership edge. They differ by
// one character and mean opposite things.
func classifyVertex(id string, links []displayLink) (vertexKind, string) {
	// The structural roots are recognised by name. They are the one part of the
	// graph whose identity is not derivable from its edges — `types` and
	// `objects` have no distinguishing links of their own, only the ones root
	// gives them.
	if builtInStructural[stripDomain(id)] {
		return vkStructural, ""
	}

	typesRoot := hubID("types")
	isType := false
	linkedFromAType := false
	typeName := ""
	fallbackTypeName := ""

	for _, dl := range links {
		if dl.isOut {
			// The instance-of edge. Named `type` by the server; a types-link
			// shares its link type but is named after its target, so the name
			// is what tells them apart. The unnamed fallback keeps older or
			// hand-built data classifiable instead of silently plain.
			if dl.info.tp == ltInstanceOf {
				if dl.info.id.name == instanceOfLinkName {
					typeName = stripDomain(dl.info.to)
				} else if fallbackTypeName == "" {
					fallbackTypeName = stripDomain(dl.info.to)
				}
			}
			continue
		}
		// Incoming.
		switch dl.info.tp {
		case ltInstanceOf:
			if dl.info.id.from == typesRoot {
				isType = true
			}
		case ltInstance:
			// From `hub/objects` or from the owning type — either way this
			// vertex is somebody's instance.
			linkedFromAType = true
		}
	}

	// A type can also carry outgoing `__type` edges (its types-links), so the
	// type test has to come first or a type with a schema link would read as an
	// object of whatever it points at.
	if isType {
		return vkType, ""
	}
	if typeName == "" {
		typeName = fallbackTypeName
	}
	if typeName != "" {
		return vkObject, typeName
	}
	if linkedFromAType {
		// Half-written: something owns it as an instance but it cannot say what
		// it is an instance OF. Naming this state is the point — it used to be
		// reported as an ordinary vertex, which is why a broken object looked
		// exactly like a plain one and the high-level API kept refusing edits
		// for no visible reason.
		return vkBrokenObject, ""
	}
	return vkPlain, ""
}

// builtInStructural is the CMDB topology proper. It is deliberately NOT the
// same set as builtInVertices (cmd_common.go): `group` and `trash_can` are
// genuine types and `nav` is a genuine object, so classifying them structurally
// would hide what they really are. They are merely undeletable, which is a
// different question and answered elsewhere.
var builtInStructural = map[string]bool{
	"root": true, "types": true, "objects": true,
}

func (m tuiModel) vertexKind() (vertexKind, string) {
	return classifyVertex(m.currentID, m.links)
}

// label names the kind for the user. Every kind gets a badge, including the
// plain one: an unbadged vertex used to mean "plain", "broken object" and
// "still loading" all at once.
func (k vertexKind) label(typeName string) string {
	switch k {
	case vkType:
		return "type"
	case vkObject:
		return "object of " + typeName
	case vkStructural:
		return "built-in"
	case vkBrokenObject:
		return "object · instance-of link missing"
	default:
		return "vertex"
	}
}

// ── Link tiers ────────────────────────────────────────────────────────────────

// linkTier is which API owns an edge — for creating it, and equally for reading,
// updating and deleting it. Routing a delete through the wrong tier is not a
// cosmetic difference: removing a types-link with the raw API leaves the schema
// gone and every instance's edge dangling, which is precisely what the CMDB
// cascade exists to prevent.
type linkTier int

const (
	tierRawLink linkTier = iota
	tierTypesLink
	tierObjectsLink
	tierSuperLink // an objects-link declared under claimed super-types
	tierSubType   // the type → sub-type edge
)

// linkTierFor decides the tier from the two endpoint kinds. With the low-level
// toggle on it is always the raw one — that is the whole point of the toggle.
func linkTierFor(from, to vertexKind, llMode bool) linkTier {
	if llMode {
		return tierRawLink
	}
	switch {
	case from == vkType && to == vkType:
		return tierTypesLink
	case from == vkObject && to == vkObject:
		return tierObjectsLink
	default:
		// A type↔object or anything involving a plain vertex has no
		// high-level meaning; the raw API is the honest choice.
		return tierRawLink
	}
}

// tierOfExistingLink routes an edge that already exists. Two things differ from
// the create case.
//
// First, ownership. The API addresses a link from its owner, and for an
// INCOMING link the owner is the far vertex — so the endpoint kinds have to be
// assigned by ownership rather than by which side happens to be on screen.
//
// Second, the edge's own type is evidence the create case does not have. Two
// type vertices can be joined by an ordinary link that is not a types-link at
// all, and an objects-link declared under claimed super-types is stored under a
// compound `from#to#rel` type that the plain objects API would resolve to a
// different edge entirely. Both are downgraded rather than guessed at.
func tierOfExistingLink(dl displayLink, nearKind, farKind vertexKind, llMode bool) linkTier {
	if llMode {
		return tierRawLink
	}
	ownerKind, targetKind := nearKind, farKind
	if !dl.isOut {
		ownerKind, targetKind = farKind, nearKind
	}

	if dl.info.tp == ltSubType && ownerKind == vkType && targetKind == vkType {
		return tierSubType
	}

	tier := linkTierFor(ownerKind, targetKind, false)
	switch tier {
	case tierTypesLink:
		// Only the instance-of-typed edge between two types is a schema link.
		if dl.info.tp != ltInstanceOf {
			return tierRawLink
		}
	case tierObjectsLink:
		if from, to, _, ok := parseSuperLinkType(dl.info.tp); ok && from != "" && to != "" {
			return tierSuperLink
		}
	}
	return tier
}

// inferFarKind names the far endpoint's kind from the edge alone.
//
// The graph's own edges are typed, and their types say what is on the other
// end: only a type is reached by `__sub`, only an object by an outgoing
// `__object`. That matters because the alternative — asking the cache, and
// degrading to the raw API when the user has not walked there — would make a
// types-link delete cascade or not depending on where the user had been, which
// is the kind of rule nobody can hold in their head.
//
// User-typed edges (an objects-link carries the schema's own link type) carry
// no such evidence, and for those the caller falls back to the cache.
func inferFarKind(dl displayLink) (vertexKind, bool) {
	switch dl.info.tp {
	case ltSubType:
		return vkType, true // type --__sub--> type
	case ltInstance:
		if dl.isOut {
			return vkObject, true // type --__object--> instance
		}
		if dl.info.id.from == hubID("objects") {
			return vkStructural, true
		}
		return vkType, true // my type claims me as its instance
	case ltInstanceOf:
		if !dl.isOut && dl.info.id.from == hubID("types") {
			return vkStructural, true
		}
		// Outgoing: my own type, or a types-link target. Incoming from
		// anywhere but the types root: somebody's types-link points at me,
		// and only a type declares one.
		return vkType, true
	case ltTypesRoot, ltObjectsRt:
		return vkStructural, true
	}
	return vkPlain, false
}

// parseSuperLinkType splits the compound link type the server writes for an
// objects-link declared under claimed super-types: "<fromClaim>#<toClaim>#<rel>".
func parseSuperLinkType(tp string) (fromClaim, toClaim, rel string, ok bool) {
	parts := strings.SplitN(tp, "#", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func (t linkTier) noun() string {
	switch t {
	case tierTypesLink:
		return "types-link"
	case tierObjectsLink:
		return "objects-link"
	case tierSuperLink:
		return "objects-link (claimed types)"
	case tierSubType:
		return "sub-type"
	default:
		return "raw link"
	}
}

func (t linkTier) title() string {
	switch t {
	case tierTypesLink:
		// Neutral, because the form asks which of the two relations this is
		// and the answer can change while it is open.
		return "Relate two types"
	case tierObjectsLink:
		return "New objects-link"
	default:
		return "New raw link (low level)"
	}
}

// ── What the armed CRUD API can act on ────────────────────────────────────────

// The status bar states which API the CRUD keys use, so the keys have to obey
// it. They did not: a plain vertex edited in high-level mode fell through to
// ops.vertexUpdate — a low-level write issued while the screen said
// "CRUD: high-level". The chip was there to end exactly that kind of guessing,
// and instead it was describing something that was not happening.
//
// A refusal names the key that makes the operation possible. Telling the user
// "no" without telling them the way to "yes" is how a mode becomes a wall.

const switchHint = " — press x to switch the CRUD API"

// crudRefusesVertex explains why the armed API cannot act on this vertex, or
// returns "" when it can.
func crudRefusesVertex(llMode bool, k vertexKind, id string) string {
	if llMode {
		// Everything is a vertex at the low level, including a type.
		return ""
	}
	switch k {
	case vkType, vkObject:
		return ""
	case vkBrokenObject:
		// The one case where low-level mode earns its keep: the high-level API
		// will refuse this vertex too, and repairing it is what the raw one is
		// for.
		return stripDomain(id) + " is an object with a broken instance-of link — " +
			"the high-level API cannot address it" + switchHint
	case vkStructural:
		return stripDomain(id) + " is part of the CMDB topology, not a type or an object" + switchHint
	default:
		return stripDomain(id) + " is a plain vertex — the high-level API only knows " +
			"types and objects" + switchHint
	}
}

// crudRefusesLink is the same question for an edge. In high-level mode only
// edges the CMDB owns are addressable; a raw link between two plain vertices
// has no high-level form at all.
func crudRefusesLink(llMode bool, t linkTier) string {
	if llMode || t != tierRawLink {
		return ""
	}
	return "this is a raw link — the high-level API only knows links between " +
		"types and between objects" + switchHint
}
