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
		kind:     formBodyEdit,
		title:    fmt.Sprintf("Edit %s body — %s", entityLabel(entity), m.currentID),
		chrome:   chromeFull,
		template: m.bodyRegister,
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
	case formLinkEdit:
		return submitLinkEditCmd(f)
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
	if refusal := crudRefusesVertex(m.llMode, kind, id); refusal != "" {
		return formState{}, "✗ " + refusal
	}
	entity := entityForVertex(kind, m.llMode)

	// Neighbours are captured NOW: after the delete lands, m.links may already
	// belong to a different vertex.
	neighbours := make([]string, 0, len(m.links))
	for _, dl := range m.links {
		neighbours = append(neighbours, dl.target())
	}

	f := formState{
		kind:    formDeleteVertex,
		chrome:  chromeStatus,
		confirm: true,
		ctx: formCtx{
			fromID:   id,
			fromKind: kind,
			fromType: typeName,
			entity:   entity,
			llMode:   m.llMode,
		},
	}

	// The title names WHAT is being deleted, not just which id — the same
	// vertex is a "type" through the CMDB API and a bare "vertex" through the
	// low-level one, and those remove very different amounts of graph.
	switch {
	case entity == entType:
		f.confirmWord = stripDomain(id)
		f.title = "DELETE TYPE " + id + " — this also removes every object of this type."
	case entity == entObject:
		f.title = "Delete OBJECT " + id + " (of " + typeName + ")?"
	case m.llMode && kind != vkPlain:
		// Standing on something the CMDB knows about, with the low-level API
		// armed. Say so: this removes the vertex and its edges, and leaves the
		// CMDB's idea of it behind.
		f.title = "Delete VERTEX " + id + " with the LOW-LEVEL API — " +
			"the CMDB " + kind.label(typeName) + " record is not cleaned up."
	default:
		f.title = "Delete vertex " + id + "?"
	}
	f.ctx.toID = strings.Join(neighbours, " ") // carried for invalidation
	return f, ""
}

// openDeleteLinkForm builds the confirmation for the link under the cursor.
//
// For an INCOMING link the owner is the other vertex, not the one on screen —
// linkId.from is already the owner, and the prompt spells the direction out so
// the user is not surprised about which side loses the edge.
// openDeleteLinkForm confirms deleting an edge, at a weight that matches what
// the deletion actually does.
//
// Deleting a schema link is not deleting one edge. The CMDB cascade walks every
// instance of the source type and removes the corresponding edge from each —
// which is precisely why removing one with the raw API is a corruption: the
// declaration disappears and every instance keeps a link the schema no longer
// permits. The TUI used to do exactly that while `cmdb typeslink delete`
// cascaded correctly, so the same action meant two different things depending
// on where you ran it. Now it cascades here too, behind the same typed-name
// confirmation a type delete demands.
func openDeleteLinkForm(m tuiModel, dl displayLink) formState {
	owner, target := linkEndpoints(dl)
	kind, _ := m.vertexKind()
	tier := m.tierOfSubjectLink(dl)

	f := formState{
		kind:    formDeleteLink,
		chrome:  chromeStatus,
		confirm: true,
		ctx: formCtx{
			fromID:   owner,
			toID:     target,
			fromKind: kind,
			linkName: dl.info.id.name,
			linkType: dl.info.tp,
			tier:     tier,
			llMode:   m.llMode,
		},
	}

	arrow := " —" + orDash(dl.info.tp) + "▶ "
	switch tier {
	case tierTypesLink:
		f.confirmWord = stripDomain(owner)
		f.title = "DELETE TYPES-LINK " + stripDomain(owner) + arrow + stripDomain(target) +
			" — this also removes that link from every object of " + stripDomain(owner) + "."
	case tierSubType:
		f.confirmWord = stripDomain(target)
		f.title = "REMOVE SUB-TYPE " + stripDomain(target) + " from " + stripDomain(owner) +
			" — every object of " + stripDomain(target) + " loses what it inherited."
	default:
		f.title = "Delete " + tier.noun() + " " + stripDomain(owner) + arrow + stripDomain(target) + "?"
	}
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
	owner, target := f.ctx.fromID, f.ctx.toID
	name := f.ctx.linkName
	tier := f.ctx.tier
	fromClaim, toClaim, _, isSuper := parseSuperLinkType(f.ctx.linkType)

	return func() tea.Msg {
		msg := mutationResultMsg{
			target:     stripDomain(owner) + " ──▶ " + stripDomain(target),
			invalidate: []string{owner, target},
			refresh:    true,
		}
		switch {
		case tier == tierTypesLink:
			msg.op, msg.res = "typeslink.delete", ops.typesLinkDelete(owner, target)
			// The cascade touches every instance of the source type. Anything
			// short of dropping the cache would be guesswork about which.
			msg.clearAll, msg.invalidate = true, nil
		case tier == tierSubType:
			msg.op, msg.res = "type.subtype.rm", ops.subTypeRemove(owner, target)
			// Removing a sub-type relation re-runs the inheritance computation
			// on arbitrary descendants.
			msg.clearAll, msg.invalidate = true, nil
		case tier == tierSuperLink && isSuper:
			msg.op, msg.res = "objectslink.super.delete",
				ops.superLinkDelete(owner, target, fromClaim, toClaim)
		case tier == tierObjectsLink:
			msg.op, msg.res = "objectslink.delete", ops.objectsLinkDelete(owner, target)
		default:
			msg.op, msg.res = "link.delete", ops.linkDelete(owner, name)
		}
		return msg
	}
}

// ── Link creation ─────────────────────────────────────────────────────────────

// openLinkCreateForm builds the link form from the anchor to the current
// vertex. The chosen API tier is in the title so the user can always see which
// one is about to run.
func openLinkCreateForm(m tuiModel) (formState, string) {
	if m.linking == nil {
		return formState{}, "press L on a source vertex first"
	}
	if m.linking.fromID == m.currentID {
		return formState{}, "the source and the target are the same vertex"
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
		},
			// The claims. An object-link is governed by the types-link between
			// the endpoints' types; naming a SUPER-type of either instead lets
			// the link be governed by a schema declared further up. That is the
			// whole of "supertype links", and the user never has to hear the
			// term: they see "link these two, treating srv-1 as machine", and
			// leaving both rows alone is the ordinary case.
			formField{
				key: "fromclaim", label: "from as", kind: fieldID,
				value: f.ctx.fromType,
				hint:  "a super-type of " + orDash(f.ctx.fromType) + " to link under, if not itself",
			}, formField{
				key: "toclaim", label: "to as", kind: fieldID,
				value: toType,
				hint:  "a super-type of " + orDash(toType) + " to link under, if not itself",
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

	fields = append(fields,
		formField{key: "tags", label: "tags", kind: fieldTags},
		formField{key: "body", label: "body", kind: fieldText,
			hint: "inline JSON, or leave blank"})
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

	name := f.value("name")
	linkType := f.value("type")
	olt := f.value("olt")
	force := f.boolean("force")

	body, bodyErr := parseInlineBody(f.value("body"))

	// The tier is settled when the form opens; recomputing it from the entity
	// here is what left `case entObject` unreachable and hid the object-object
	// case inside a nested condition in the default arm.
	tier := linkTierFor(f.ctx.fromKind, f.ctx.toKind, f.ctx.llMode)

	// A claim that names something other than the endpoint's own type means
	// the link is governed by a schema declared further up the hierarchy.
	fromClaim, toClaim := f.value("fromclaim"), f.value("toclaim")
	claimed := tier == tierObjectsLink &&
		((fromClaim != "" && fromClaim != f.ctx.fromType) ||
			(toClaim != "" && toClaim != f.ctx.toType))

	return func() tea.Msg {
		if bodyErr != "" {
			return mutationResultMsg{
				op: "link.create", target: stripDomain(from) + " → " + stripDomain(to),
				res: opResult{status: opFailed, details: bodyErr},
			}
		}
		var (
			res opResult
			op  string
		)
		switch {
		case tier == tierTypesLink:
			op = "typeslink.create"
			res = ops.typesLinkCreate(from, to, olt, tags, body)
		case claimed:
			op = "objectslink.super.create"
			res = ops.superLinkCreate(from, to, orElse(fromClaim, f.ctx.fromType),
				orElse(toClaim, f.ctx.toType), name, tags, body)
		case tier == tierObjectsLink:
			op = "objectslink.create"
			res = ops.objectsLinkCreate(from, to, name, tags, body)
		default:
			op = "link.create"
			res = ops.linkCreate(from, to, name, linkType, tags, body, force)
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

// parseInlineBody turns a one-line JSON field into a body. Blank means an empty
// object, which is what every link create used to send unconditionally.
func parseInlineBody(s string) (easyjson.JSON, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return easyjson.NewJSONObject(), ""
	}
	j, ok := easyjson.JSONFromString(s)
	if !ok || !j.IsObject() {
		return easyjson.NewJSONObject(), "body is not a JSON object: " + s
	}
	return j, ""
}

func orElse(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
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
	typesRoot := hubID("types")

	linkLabel := "link — start here, walk to the target, press L again"
	if m.linking != nil {
		linkLabel = "link — commit: " + stripDomain(m.linking.fromID) + " ──▶ " + bare
	}

	var out []menuEntry

	// High-level entities need the high-level API. A create menu that offered
	// "type" while the status bar said CRUD: low-level would be describing an
	// operation that is not the one about to run.
	if m.llMode {
		return []menuEntry{
			{key: "l", label: linkLabel, kind: formLinkCreate},
			{key: "v", label: "raw vertex — linked from " + bare, kind: formVertexCreate},
			{key: "t", label: "type", why: "types are a high-level entity" + switchHint,
				kind: formTypeCreate},
			{key: "o", label: "object", why: "objects are a high-level entity" + switchHint,
				kind: formObjectCreate},
		}
	}

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
			unavailableAt: canonID(typeName),
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

	// A raw vertex is a low-level entity, so it needs the low-level API — the
	// same rule that stops a type being created in low-level mode, applied in
	// the other direction. When it IS available it must be attached to the
	// vertex it is created from, otherwise nothing in the graph points at it
	// and the browser can never reach it again.
	out = append(out, menuEntry{
		key: "v", label: "raw vertex",
		why:  "a raw vertex is a low-level entity" + switchHint,
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
			// Shown bare because that is how the user thinks of it; the
			// canonical form the cache is keyed by comes from ctx.fromID.
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

func submitCreateCmd(f formState) tea.Cmd {
	// Ids are canonicalised before they go anywhere near the cache. The wire
	// call still gets the bare name — the server resolves it — but `navTo` and
	// `invalidate` must name the vertex the way the cache keys it, or the
	// eviction silently misses and the browser keeps serving the link list it
	// captured before the write. That is the whole of "I created an object and
	// there is no link from its type to it": the link existed, the screen was
	// a snapshot from before it did.
	dom := f.ctx.domain
	if dom == "" {
		dom = NatsHubDomain
	}

	switch f.kind {
	case formVertexCreate:
		id := f.value("id")
		canon := canonIDIn(id, dom)
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
					invalidate: []string{canon, from}, refresh: true,
				}
			}
			return mutationResultMsg{
				op: "vertex.create", target: id, res: res,
				invalidate: []string{canon, from},
				navTo:      canon, // land on what was just made
			}
		}
	case formTypeCreate:
		name := f.value("name")
		// Type operations are redirected to the hub wherever the user browses,
		// so a type is always a hub vertex regardless of where it was created.
		canon := canonID(name)
		return func() tea.Msg {
			res := ops.typeCreate(name, easyjson.NewJSONObject())
			msg := mutationResultMsg{
				op: "type.create", target: name, res: res,
				invalidate: []string{canon, hubID("types")},
			}
			if res.status != opFailed {
				msg.navTo = canon
			} else {
				msg.refresh = true
			}
			return msg
		}
	case formObjectCreate:
		id, tp := f.value("id"), f.value("type")
		canon := canonIDIn(id, dom)
		// ctx.fromID is the type vertex the user is standing on, already in
		// canonical form — which is exactly the cache entry that has to be
		// evicted for the new instance to show up on it.
		typeCanon := f.ctx.fromID
		return func() tea.Msg {
			res := ops.objectCreate(id, tp, easyjson.NewJSONObject())
			msg := mutationResultMsg{
				op: "object.create", target: id, res: res,
				invalidate: []string{canon, typeCanon, hubID("objects")},
			}
			if res.status != opFailed {
				msg.navTo = canon
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

// ── Link edit ─────────────────────────────────────────────────────────────────

// entityForTier picks which body paths are machine-owned on an edge. This is
// what finally makes entTypesLink reachable: a types-link's `body.type` is the
// object-link type its instances inherit, so a REPLACE that dropped it would
// silently rewrite the schema. The protection was written long ago and no code
// path ever produced the entity that switches it on.
func entityForTier(t linkTier) entityKind {
	if t == tierTypesLink {
		return entTypesLink
	}
	return entLink
}

// openLinkEditForm edits an existing edge: its tags and its body, together.
//
// Together, because the server applies `replace` to BOTH with a single flag —
// two controls for one flag is a lie, and the previous tags-only form told it
// by hardcoding an empty body, so ticking `replace` to clear tags also wiped
// the body nobody could see.
//
// The detail must already be loaded. Seeding from the model is what produced a
// blank tags field over a link that had tags.
func openLinkEditForm(m tuiModel, dl displayLink, focusKey string) (formState, string) {
	detail, loaded := m.linkDetailFor(dl)
	if !loaded {
		return formState{}, "reading the link…"
	}
	if detail.err != nil {
		return formState{}, "✗ cannot read link: " + detail.err.Error()
	}

	owner, target := linkEndpoints(dl)
	tier := m.tierOfSubjectLink(dl)
	entity := entityForTier(tier)

	w, h := m.editorSizeFor(len(linkEditContextRows(dl, tier)) + 2)
	ed := newJSONEditor(detail.body, entity, w, h)

	f := formState{
		kind:        formLinkEdit,
		title:       "Edit " + tier.noun() + " — " + stripDomain(owner) + " ──▶ " + stripDomain(target),
		chrome:      chromeFull,
		contextRows: linkEditContextRows(dl, tier),
		template:    m.bodyRegister,
		ctx: formCtx{
			fromID:   owner,
			toID:     target,
			linkName: dl.info.id.name,
			linkType: dl.info.tp,
			tier:     tier,
			entity:   entity,
			origBody: detail.body,
			llMode:   m.llMode,
		},
		fields: []formField{
			{key: "tags", label: "tags", kind: fieldTags,
				value: strings.Join(detail.tags, ", "),
				hint:  "space or comma separated"},
			{key: "body", label: "body", kind: fieldJSON, json: ed},
		},
	}
	if focusKey == "tags" {
		f.cur = 0
	} else {
		f.cur = 1
	}
	return f.validate(), ""
}

// linkEditContextRows are the facts that identify the edge but cannot be
// changed: the API addresses a link BY its owner and name, and moving an
// endpoint is a delete plus a create, not an update. They are shown rather
// than omitted because without them the form does not say which of several
// same-named edges it is about to write.
func linkEditContextRows(dl displayLink, tier linkTier) []string {
	owner, target := linkEndpoints(dl)
	rows := []string{
		"endpoints  " + stripDomain(owner) + " ──▶ " + stripDomain(target),
		"name       " + dl.info.id.name,
		"type       " + orDash(dl.info.tp),
		"via        " + tier.noun(),
	}
	if from, to, rel, ok := parseSuperLinkType(dl.info.tp); ok {
		rows = append(rows, "as         "+from+" ──▶ "+to+"   rel "+rel)
	}
	return rows
}

func submitLinkEditCmd(f formState) tea.Cmd {
	owner, target := f.ctx.fromID, f.ctx.toID
	name := f.ctx.linkName
	tags := f.tags("tags")
	body := bodyOrEmpty(f, "body")
	ed := f.jsonField()
	replace := ed != nil && ed.replace
	tier := f.ctx.tier
	fromClaim, toClaim, _, isSuper := parseSuperLinkType(f.ctx.linkType)

	return func() tea.Msg {
		var (
			res opResult
			op  string
		)
		switch {
		case tier == tierTypesLink:
			op = "typeslink.update"
			res = ops.typesLinkUpdate(owner, target, tags, body, replace)
		case tier == tierSuperLink && isSuper:
			op = "objectslink.super.update"
			res = ops.superLinkUpdate(owner, target, fromClaim, toClaim, name, tags, body, replace)
		case tier == tierObjectsLink:
			op = "objectslink.update"
			res = ops.objectsLinkUpdate(owner, target, tags, body, replace)
		default:
			op = "link.update"
			res = ops.linkUpdate(owner, name, tags, body, replace)
		}
		return mutationResultMsg{
			op: op, target: stripDomain(owner) + " ──▶ " + stripDomain(target),
			res: res, invalidate: []string{owner, target}, refresh: true,
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
