package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/foliagecp/easyjson"
)

// Per-entity form construction and submission.
//
// Constructors are pure — they read a snapshot of the model and return a
// formState. Submission returns a tea.Cmd that calls exactly one graphOps
// function and reports back as a mutationResultMsg.

// ── Body edit ─────────────────────────────────────────────────────────────────

// entityForVertex maps a classified vertex to the API tier its body edit uses.
// With the low-level toggle on, everything is a raw vertex.
func entityForVertex(k vertexKind, llMode bool) entityKind {
	if llMode {
		return entVertex
	}
	switch k {
	case vkType:
		return entType
	case vkObject:
		return entObject
	default:
		return entVertex
	}
}

// openBodyEditForm builds the editor for the current vertex's body.
func openBodyEditForm(m tuiModel) (formState, bool) {
	if m.fvi == nil || m.fvi.body == nil {
		return formState{}, false
	}
	kind, typeName := m.vertexKind()
	entity := entityForVertex(kind, m.llMode)

	w, h := m.editorSize()
	ed := newJSONEditor(*m.fvi.body, entity, w, h)

	f := formState{
		kind:   formBodyEdit,
		title:  fmt.Sprintf("Edit %s body — %s", entityLabel(entity), m.currentID),
		chrome: chromeFull,
		ctx: formCtx{
			fromID:   m.currentID,
			fromKind: kind,
			fromType: typeName,
			origBody: *m.fvi.body,
			entity:   entity,
			llMode:   m.llMode,
		},
		fields: []formField{{
			key:   "body",
			label: "body",
			kind:  fieldJSON,
			json:  ed,
		}},
	}
	return f.validate(), true
}

// submitFormCmd runs the form's operation off the UI goroutine.
func submitFormCmd(f formState) tea.Cmd {
	switch f.kind {
	case formBodyEdit:
		return submitBodyEditCmd(f)
	case formDeleteVertex:
		return submitDeleteVertexCmd(f)
	case formDeleteLink:
		return submitDeleteLinkCmd(f)
	case formLinkCreate:
		return submitLinkCreateCmd(f)
	case formLinkTags:
		return submitLinkTagsCmd(f)
	case formVertexCreate, formTypeCreate, formObjectCreate, formSubTypeSet:
		return submitCreateCmd(f)
	}
	return nil
}

func submitBodyEditCmd(f formState) tea.Cmd {
	body, ok := f.body("body")
	if !ok {
		return func() tea.Msg {
			return mutationResultMsg{
				op:     "body.update",
				target: f.ctx.fromID,
				res:    opResult{status: opFailed, details: "body is not a JSON object"},
			}
		}
	}
	ed := f.fields[0].json
	replace := ed != nil && ed.replace

	id := f.ctx.fromID
	entity := f.ctx.entity

	return func() tea.Msg {
		var (
			res opResult
			op  string
		)
		switch entity {
		case entType:
			op = "type.update"
			res = ops.typeUpdate(id, body, replace, false)
		case entObject:
			op = "object.update"
			res = ops.objectUpdate(id, body, replace, "")
		default:
			op = "vertex.update"
			res = ops.vertexUpdate(id, body, replace, false)
		}
		return mutationResultMsg{
			op:         op,
			target:     id,
			res:        res,
			invalidate: []string{id},
			refresh:    true,
		}
	}
}

// ── Pending link ──────────────────────────────────────────────────────────────

// pendingLink is a link-in-progress: the source has been chosen and the user is
// now navigating to the target.
//
// It is deliberately a two-step commit rather than a form that asks for both
// endpoints. Typing a target id defeats the point of having a graph browser —
// the whole reason to be in the TUI is that you find things by walking to them.
// So: Start here, walk anywhere, Commit there.
//
// It is DATA, not a mode: every navigation key keeps working while it is set,
// which is what makes "walk anywhere" true. What makes it discoverable is the
// banner it puts in the breadcrumb row for as long as it is pending.
type pendingLink struct {
	fromID   string
	kind     vertexKind
	typeName string
}

// ── Delete ────────────────────────────────────────────────────────────────────

// deleteTier grades how much confirmation a deletion needs.
type deleteTier int

const (
	tierBlocked  deleteTier = iota // structural vertex: refused outright
	tierOrdinary                   // single-key y/n
	tierCascade                    // type the name, and show the blast radius
)

// openDeleteVertexForm builds the confirmation for deleting the current vertex.
//
// The tier depends on what the vertex is: deleting a TYPE walks every instance
// and deletes it too, which is a different order of consequence from deleting
// one object, and the confirmation should not pretend otherwise.
func openDeleteVertexForm(m tuiModel) (formState, string) {
	id := m.currentID
	if builtInVertices[stripDomain(id)] {
		return formState{}, "✗ " + id + " is a built-in vertex and cannot be deleted"
	}

	kind, typeName := m.vertexKind()
	entity := entityForVertex(kind, m.llMode)

	// Neighbours are captured NOW: after the delete lands, m.links may already
	// belong to a different vertex.
	neighbours := make([]string, 0, len(m.links))
	for _, dl := range m.links {
		neighbours = append(neighbours, dl.target())
	}

	f := formState{
		kind:     formDeleteVertex,
		chrome:   chromeStatus,
		confirm:  true,
		affected: -1,
		ctx: formCtx{
			fromID:   id,
			fromKind: kind,
			fromType: typeName,
			entity:   entity,
			llMode:   m.llMode,
		},
	}

	if entity == entType {
		f.confirmWord = stripDomain(id)
		f.title = "DELETE TYPE " + id + " — this also removes every object of this type."
	} else {
		f.title = "Delete " + entityLabel(entity) + " " + id + "?"
	}
	f.ctx.toID = strings.Join(neighbours, " ") // carried for invalidation
	return f, ""
}

// openDeleteLinkForm builds the confirmation for the link under the cursor.
//
// For an INCOMING link the owner is the other vertex, not the one on screen —
// linkId.from is already the owner, and the prompt spells the direction out so
// the user is not surprised about which side loses the edge.
func openDeleteLinkForm(m tuiModel, dl displayLink) formState {
	owner := dl.info.id.from
	kind, _ := m.vertexKind()

	f := formState{
		kind:     formDeleteLink,
		chrome:   chromeStatus,
		confirm:  true,
		affected: -1,
		ctx: formCtx{
			fromID:   owner,
			toID:     dl.info.to,
			fromKind: kind,
			linkName: dl.info.id.name,
			linkType: dl.info.tp,
			llMode:   m.llMode,
		},
	}
	arrow := " —" + dl.info.tp + "▶ "
	f.title = "Delete link " + stripDomain(owner) + arrow + stripDomain(dl.info.to) + "?"
	return f
}

func submitDeleteVertexCmd(f formState) tea.Cmd {
	id := f.ctx.fromID
	entity := f.ctx.entity
	neighbours := strings.Fields(f.ctx.toID)

	return func() tea.Msg {
		var (
			res opResult
			op  string
		)
		switch entity {
		case entType:
			op = "type.delete"
			res = ops.typeDelete(id)
		case entObject:
			op = "object.delete"
			res = ops.objectDelete(id)
		default:
			op = "vertex.delete"
			res = ops.vertexDelete(id)
		}

		msg := mutationResultMsg{
			op:          op,
			target:      id,
			res:         res,
			clearLinkIf: id,
		}
		if entity == entType {
			// A type delete cascades into an unbounded number of object
			// deletes; anything short of clearing the cache would be guesswork.
			msg.clearAll = true
		} else {
			msg.invalidate = append([]string{id}, neighbours...)
		}
		return msg
	}
}

func submitDeleteLinkCmd(f formState) tea.Cmd {
	from, to := f.ctx.fromID, f.ctx.toID
	name := f.ctx.linkName

	return func() tea.Msg {
		res := ops.linkDelete(from, name)
		return mutationResultMsg{
			op:         "link.delete",
			target:     from + ":" + name,
			res:        res,
			invalidate: []string{from, to},
			refresh:    true,
		}
	}
}

// ── Link creation ─────────────────────────────────────────────────────────────

// linkTier is which API a new link should go through.
type linkTier int

const (
	tierRawLink linkTier = iota
	tierTypesLink
	tierObjectsLink
)

// linkTierFor decides the API from the two endpoints. With the low-level
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

func (t linkTier) title() string {
	switch t {
	case tierTypesLink:
		return "New types-link"
	case tierObjectsLink:
		return "New objects-link"
	default:
		return "New raw link (low level)"
	}
}

// openLinkCreateForm builds the link form from the anchor to the current
// vertex. The chosen API tier is in the title so the user can always see which
// one is about to run.
func openLinkCreateForm(m tuiModel) (formState, string) {
	if m.linking == nil {
		return formState{}, "press a to anchor a source vertex first"
	}
	if m.linking.fromID == m.currentID {
		return formState{}, "the anchor and the target are the same vertex"
	}

	toKind, toType := m.vertexKind()
	tier := linkTierFor(m.linking.kind, toKind, m.llMode)

	f := formState{
		kind:   formLinkCreate,
		title:  tier.title(),
		chrome: chromeCenter,
		ctx: formCtx{
			fromID:   m.linking.fromID,
			toID:     m.currentID,
			fromKind: m.linking.kind,
			toKind:   toKind,
			fromType: m.linking.typeName,
			toType:   toType,
			entity:   entLink,
			llMode:   m.llMode,
		},
	}

	fields := []formField{{
		key:   "ends",
		label: "endpoints",
		kind:  fieldVertex,
	}}

	switch tier {
	case tierTypesLink:
		f.ctx.entity = entTypesLink
		fields = append(fields, formField{
			key: "olt", label: "object link type", kind: fieldID, required: true,
			// Naming it after the target type is the convention the server
			// itself falls back to for link names.
			value: stripDomain(m.currentID),
			hint:  "the link type that object-links between instances will get",
		})
	case tierObjectsLink:
		fields = append(fields, formField{
			key: "name", label: "name", kind: fieldID,
			value: stripDomain(m.currentID),
			hint:  "blank lets the server name it after the target",
		}, formField{
			key: "ltype", label: "link type", kind: fieldStatic,
			static: "(derived from the types-link — checked on submit)",
		})
	default:
		fields = append(fields, formField{
			key: "name", label: "name", kind: fieldID, required: true,
			value: stripDomain(m.currentID),
		}, formField{
			key: "type", label: "type", kind: fieldID, required: true,
			value: lastLinkTypeOn(m, m.linking.fromID),
		}, formField{
			key: "force", label: "force", kind: fieldBool,
			hint: "overwrite an existing link, bypassing the uniqueness checks",
		})
	}

	fields = append(fields, formField{key: "tags", label: "tags", kind: fieldTags})
	f.fields = fields
	f.cur = 1 // the endpoints row is informational; start on the first input

	// Local pre-checks against what we already know, so the user is told about
	// a clash before a round trip rather than by an opaque server error.
	f = f.validate()
	return f, ""
}

// lastLinkTypeOn guesses a link type from what the source vertex already uses,
// which is right far more often than an empty field.
func lastLinkTypeOn(m tuiModel, from string) string {
	for _, dl := range m.links {
		if dl.isOut && dl.info.id.from == from && dl.info.tp != "" &&
			!strings.HasPrefix(dl.info.tp, "__") {
			return dl.info.tp
		}
	}
	return ""
}

func submitLinkCreateCmd(f formState) tea.Cmd {
	from, to := f.ctx.fromID, f.ctx.toID
	tags := f.tags("tags")
	entity := f.ctx.entity

	name := f.value("name")
	linkType := f.value("type")
	olt := f.value("olt")
	force := f.boolean("force")

	return func() tea.Msg {
		var (
			res opResult
			op  string
		)
		switch entity {
		case entTypesLink:
			op = "typeslink.create"
			res = ops.typesLinkCreate(from, to, olt, tags, easyjson.NewJSONObject())
		case entObject:
			op = "objectslink.create"
			res = ops.objectsLinkCreate(from, to, name, tags, easyjson.NewJSONObject())
		default:
			if entity == entLink && f.ctx.fromKind == vkObject && f.ctx.toKind == vkObject && !f.ctx.llMode {
				op = "objectslink.create"
				res = ops.objectsLinkCreate(from, to, name, tags, easyjson.NewJSONObject())
			} else {
				op = "link.create"
				res = ops.linkCreate(from, to, name, linkType, tags, easyjson.NewJSONObject(), force)
			}
		}
		return mutationResultMsg{
			op:         op,
			target:     stripDomain(from) + " → " + stripDomain(to),
			res:        res,
			invalidate: []string{from, to},
			refresh:    true,
		}
	}
}

// ── Create menu ───────────────────────────────────────────────────────────────

// The governing rule for creation: YOU CREATE A THING WHERE THAT THING LIVES.
//
// Types are created from the `types` root, objects from their own type, links
// from one of their endpoints. This is not a stylistic choice — it is what
// makes the TUI's promise hold. The TUI finds things by walking the graph, so
// anything created outside its home would be unreachable by the very tool that
// made it. Gating creation on location means a new entity is always attached
// to something you can already see.
//
// Where an action is not available, the menu still SHOWS it, says where it
// does live, and pressing it takes you there. A menu that silently omits
// options teaches nothing; one that explains teaches the data model.

type menuEntry struct {
	key   string
	label string

	// unavailableAt, when set, is where this action does live. The entry is
	// dimmed and choosing it navigates there instead of acting.
	unavailableAt string
	why           string

	kind formKind
}

func (e menuEntry) available() bool { return e.unavailableAt == "" && e.why == "" }

// createMenuFor lists what can be created from where the user is standing.
func createMenuFor(m tuiModel) []menuEntry {
	kind, typeName := m.vertexKind()
	bare := stripDomain(m.currentID)
	typesRoot := NatsHubDomain + "/types"

	linkLabel := "link — start here, walk to the target, press L again"
	if m.linking != nil {
		linkLabel = "link — commit: " + stripDomain(m.linking.fromID) + " ──▶ " + bare
	}

	var out []menuEntry

	// Types live under the types root.
	if bare == "types" {
		out = append(out, menuEntry{key: "t", label: "type", kind: formTypeCreate})
	} else {
		out = append(out, menuEntry{
			key: "t", label: "type",
			unavailableAt: typesRoot,
			why:           "types are created from " + typesRoot,
			kind:          formTypeCreate,
		})
	}

	// Objects live under their type.
	switch {
	case kind == vkType:
		out = append(out, menuEntry{key: "o", label: "object of " + bare, kind: formObjectCreate})
		out = append(out, menuEntry{key: "s", label: "sub-type of " + bare, kind: formSubTypeSet})
	case kind == vkObject && typeName != "":
		out = append(out, menuEntry{
			key: "o", label: "object of " + typeName,
			unavailableAt: typeName,
			why:           "objects are created from their type",
			kind:          formObjectCreate,
		})
	default:
		out = append(out, menuEntry{
			key: "o", label: "object",
			unavailableAt: typesRoot,
			why:           "objects are created from their type — pick one under " + typesRoot,
			kind:          formObjectCreate,
		})
	}

	// Links live on their endpoints, so they can always be started.
	out = append(out, menuEntry{key: "l", label: linkLabel, kind: formLinkCreate})

	// A raw vertex has no home of its own, which is exactly why it must be
	// attached to the vertex it is created from — otherwise nothing in the
	// graph points at it and the browser can never reach it again.
	out = append(out, menuEntry{
		key: "v", label: "raw vertex — linked from " + bare + " (low level)",
		kind: formVertexCreate,
	})

	return out
}

func openCreateMenu(m tuiModel) formState {
	entries := createMenuFor(m)
	fields := make([]formField, len(entries))
	for i, e := range entries {
		label := e.label
		if !e.available() {
			label += "   ⟶ " + e.why
		}
		fields[i] = formField{key: e.key, label: e.key, kind: fieldStatic, static: label}
	}
	return formState{
		kind:   formCreateMenu,
		title:  "New… (from " + stripDomain(m.currentID) + ")",
		chrome: chromeCenter,
		fields: fields,
	}
}

// ── Vertex / type / object / sub-type creation ────────────────────────────────

// openVertexCreateForm builds the raw-vertex form.
//
// The link back to the current vertex is NOT optional and has no toggle: a raw
// vertex nothing points at is invisible to a graph browser the moment you
// navigate away. Creating one would be handing the user a lost object.
func openVertexCreateForm(m tuiModel) formState {
	dom := domainOf(m.currentID)
	f := formState{
		kind:   formVertexCreate,
		title:  "New raw vertex, linked from " + stripDomain(m.currentID),
		chrome: chromeCenter,
		ctx:    formCtx{fromID: m.currentID, entity: entVertex, domain: dom},
		fields: []formField{
			{key: "id", label: "id", kind: fieldID, required: true,
				hint: "will create " + dom + "/<id>"},
			{key: "linkname", label: "link name", kind: fieldID, required: true,
				hint: "the link " + stripDomain(m.currentID) + " ──▶ <id> that keeps it reachable"},
			{key: "linktype", label: "link type", kind: fieldID, required: true,
				value: lastLinkTypeOn(m, m.currentID)},
		},
	}
	return f.validate()
}

func openTypeCreateForm(m tuiModel) formState {
	f := formState{
		kind:   formTypeCreate,
		title:  "New type",
		chrome: chromeCenter,
		ctx:    formCtx{entity: entType},
		fields: []formField{
			{key: "name", label: "name", kind: fieldID, required: true,
				// Type operations are redirected to the hub wherever the user
				// is browsing, so the preview must not promise a local domain.
				hint: "will create " + NatsHubDomain + "/<name>, linked under " +
					NatsHubDomain + "/types"},
		},
	}
	return f.validate()
}

// openObjectCreateForm is only reachable while standing on a type, so the type
// is settled and shown read-only rather than offered as a field to mistype.
func openObjectCreateForm(m tuiModel) formState {
	dom := domainOf(m.currentID)
	f := formState{
		kind:   formObjectCreate,
		title:  "New object of " + stripDomain(m.currentID),
		chrome: chromeCenter,
		ctx:    formCtx{fromID: m.currentID, entity: entObject, domain: dom},
		fields: []formField{
			{key: "id", label: "id", kind: fieldID, required: true,
				hint: "will create " + dom + "/<id>"},
			{key: "type", label: "type", kind: fieldStatic, static: stripDomain(m.currentID)},
		},
	}
	return f.validate()
}

// openSubTypeForm declares another type a sub-type of the one in view.
func openSubTypeForm(m tuiModel) formState {
	f := formState{
		kind:   formSubTypeSet,
		title:  "Declare a sub-type of " + stripDomain(m.currentID),
		chrome: chromeCenter,
		ctx:    formCtx{fromID: m.currentID, entity: entType},
		fields: []formField{
			{key: "child", label: "child type", kind: fieldID, required: true,
				hint: "an existing type that inherits from " + stripDomain(m.currentID)},
		},
	}
	return f.validate()
}

func domainOf(id string) string {
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	return NatsHubDomain
}

func submitCreateCmd(f formState) tea.Cmd {
	switch f.kind {
	case formVertexCreate:
		id := f.value("id")
		from := f.ctx.fromID
		linkName, linkType := f.value("linkname"), f.value("linktype")
		return func() tea.Msg {
			res := ops.vertexCreate(id, easyjson.NewJSONObject())
			if res.status == opFailed {
				return mutationResultMsg{op: "vertex.create", target: id, res: res}
			}
			// The vertex exists but is unreachable until this lands, so a
			// failure here is reported as the failure of the whole operation
			// rather than as a successful create.
			linkRes := ops.linkCreate(from, id, linkName, linkType, nil, easyjson.NewJSONObject(), false)
			if linkRes.status == opFailed {
				linkRes.details = "vertex created but linking it failed: " + linkRes.details
				return mutationResultMsg{
					op: "vertex.create", target: id, res: linkRes,
					invalidate: []string{id, from}, refresh: true,
				}
			}
			return mutationResultMsg{
				op: "vertex.create", target: id, res: res,
				invalidate: []string{id, from},
				navTo:      id, // land on what was just made
			}
		}
	case formTypeCreate:
		name := f.value("name")
		return func() tea.Msg {
			res := ops.typeCreate(name, easyjson.NewJSONObject())
			msg := mutationResultMsg{
				op: "type.create", target: name, res: res,
				invalidate: []string{name, NatsHubDomain + "/types"},
			}
			if res.status != opFailed {
				msg.navTo = name
			} else {
				msg.refresh = true
			}
			return msg
		}
	case formObjectCreate:
		id, tp := f.value("id"), f.value("type")
		return func() tea.Msg {
			res := ops.objectCreate(id, tp, easyjson.NewJSONObject())
			msg := mutationResultMsg{
				op: "object.create", target: id, res: res,
				invalidate: []string{id, tp, NatsHubDomain + "/objects"},
			}
			if res.status != opFailed {
				msg.navTo = id
			} else {
				msg.refresh = true
			}
			return msg
		}
	case formSubTypeSet:
		base, child := f.ctx.fromID, f.value("child")
		return func() tea.Msg {
			return mutationResultMsg{
				op: "type.subtype.add", target: stripDomain(base) + " → " + child,
				res: ops.subTypeSet(base, child),
				// The inheritance recompute rewrites cached parent lists on
				// arbitrary descendants, so nothing cached can be trusted.
				clearAll: true,
				refresh:  true,
			}
		}
	}
	return nil
}

// ── Link tags ─────────────────────────────────────────────────────────────────

func openLinkTagsForm(m tuiModel, dl displayLink) formState {
	f := formState{
		kind:   formLinkTags,
		title:  "Tags of " + dl.info.id.name,
		chrome: chromeCenter,
		ctx: formCtx{
			fromID:   dl.info.id.from,
			toID:     dl.info.to,
			linkName: dl.info.id.name,
			linkType: dl.info.tp,
			entity:   entLink,
		},
		fields: []formField{
			{key: "tags", label: "tags", kind: fieldTags,
				value: strings.Join(dl.info.tags, ", "),
				hint:  "space or comma separated"},
			{key: "replace", label: "replace", kind: fieldBool,
				hint: "REQUIRED to clear tags — a merge can only add them"},
		},
	}
	return f.validate()
}

func submitLinkTagsCmd(f formState) tea.Cmd {
	from, name := f.ctx.fromID, f.ctx.linkName
	to := f.ctx.toID
	tags := f.tags("tags")
	replace := f.boolean("replace")

	return func() tea.Msg {
		res := ops.linkUpdate(from, name, tags, easyjson.NewJSONObject(), replace)
		return mutationResultMsg{
			op: "link.tags", target: from + ":" + name,
			res: res, invalidate: []string{from, to}, refresh: true,
		}
	}
}

// ── Toast rendering ───────────────────────────────────────────────────────────

// toastFor renders the outcome of a mutation for the status line. Applied and
// no-op are visually distinct on purpose: the client reports nil for both, and
// showing success either way would misinform.
func toastFor(msg mutationResultMsg) string {
	switch msg.res.status {
	case opApplied:
		return styleOk.Render("✓ " + msg.op + "  " + stripDomain(msg.target))
	case opNoop:
		detail := msg.res.details
		if detail == "" {
			detail = "already in that state"
		}
		return styleDim.Render("∅ " + msg.op + "  " + stripDomain(msg.target) + " — " + detail)
	default:
		return styleErr.Render("✗ " + msg.op + "  " + msg.res.details)
	}
}

// bodyOrEmpty is a small helper for flows that may have no body field.
func bodyOrEmpty(f formState, key string) easyjson.JSON {
	if b, ok := f.body(key); ok {
		return b
	}
	return easyjson.NewJSONObject()
}
