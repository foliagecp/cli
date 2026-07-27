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
	case formVertexCreate, formTypeCreate, formObjectCreate:
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

// ── Anchor ────────────────────────────────────────────────────────────────────

// anchorState marks a vertex as the source for the next link. It is data, not
// a mode: every navigation key keeps working while it is set.
type anchorState struct {
	id       string
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
			op:            op,
			target:        id,
			res:           res,
			clearAnchorIf: id,
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
	if m.anchor == nil {
		return formState{}, "press a to anchor a source vertex first"
	}
	if m.anchor.id == m.currentID {
		return formState{}, "the anchor and the target are the same vertex"
	}

	toKind, toType := m.vertexKind()
	tier := linkTierFor(m.anchor.kind, toKind, m.llMode)

	f := formState{
		kind:   formLinkCreate,
		title:  tier.title(),
		chrome: chromeCenter,
		ctx: formCtx{
			fromID:   m.anchor.id,
			toID:     m.currentID,
			fromKind: m.anchor.kind,
			toKind:   toKind,
			fromType: m.anchor.typeName,
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
			value: lastLinkTypeOn(m, m.anchor.id),
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

type menuEntry struct {
	key      string
	label    string
	disabled string // non-empty ⇒ shown dimmed with this reason
	kind     formKind
}

// createMenuFor lists what can be created from where the user is standing.
// Entries that need an anchor are shown dimmed with the reason rather than
// hidden — hiding them teaches the user nothing.
func createMenuFor(m tuiModel) []menuEntry {
	kind, typeName := m.vertexKind()
	anchored := m.anchor != nil

	needAnchor := ""
	if !anchored {
		needAnchor = "press a to anchor a source vertex first"
	}

	var out []menuEntry
	switch {
	case kind == vkType && !m.llMode:
		out = append(out,
			menuEntry{"o", "new object of " + stripDomain(m.currentID), "", formObjectCreate},
			menuEntry{"l", "new link from ⚓ to here", needAnchor, formLinkCreate},
			menuEntry{"t", "new type", "", formTypeCreate},
		)
	case kind == vkObject && !m.llMode:
		out = append(out,
			menuEntry{"l", "new link from ⚓ to here", needAnchor, formLinkCreate},
			menuEntry{"o", "new object of " + typeName, "", formObjectCreate},
			menuEntry{"t", "new type", "", formTypeCreate},
		)
	default:
		out = append(out,
			menuEntry{"v", "new raw vertex", "", formVertexCreate},
			menuEntry{"l", "new raw link from ⚓ to here", needAnchor, formLinkCreate},
			menuEntry{"t", "new type", "", formTypeCreate},
		)
	}
	return out
}

func openCreateMenu(m tuiModel) formState {
	entries := createMenuFor(m)
	fields := make([]formField, len(entries))
	for i, e := range entries {
		label := e.label
		if e.disabled != "" {
			label += "  — " + e.disabled
		}
		fields[i] = formField{
			key: e.key, label: e.key, kind: fieldStatic, static: label,
		}
	}
	return formState{
		kind:   formCreateMenu,
		title:  "New…",
		chrome: chromeCenter,
		fields: fields,
	}
}

// ── Vertex / type / object creation ───────────────────────────────────────────

func openVertexCreateForm(m tuiModel) formState {
	f := formState{
		kind:   formVertexCreate,
		title:  "New raw vertex",
		chrome: chromeCenter,
		ctx:    formCtx{entity: entVertex, domain: domainOf(m.currentID)},
		fields: []formField{
			{key: "id", label: "id", kind: fieldID, required: true,
				hint: "will create " + domainOf(m.currentID) + "/<id>"},
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
				// Type operations are redirected to the hub regardless of
				// where the user is browsing.
				hint: "will create " + NatsHubDomain + "/<name>"},
		},
	}
	return f.validate()
}

func openObjectCreateForm(m tuiModel) formState {
	kind, typeName := m.vertexKind()

	typeField := formField{key: "type", label: "type", kind: fieldID, required: true}
	if kind == vkType {
		// Standing on the type is the whole point of context sensitivity: the
		// type is settled, so it is shown but not editable.
		typeField = formField{key: "type", label: "type", kind: fieldStatic,
			static: stripDomain(m.currentID)}
	} else if typeName != "" {
		typeField.value = typeName
	}

	f := formState{
		kind:   formObjectCreate,
		title:  "New object",
		chrome: chromeCenter,
		ctx:    formCtx{entity: entObject, domain: domainOf(m.currentID)},
		fields: []formField{
			{key: "id", label: "id", kind: fieldID, required: true,
				hint: "will create " + domainOf(m.currentID) + "/<id>"},
			typeField,
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
		return func() tea.Msg {
			return mutationResultMsg{
				op: "vertex.create", target: id,
				res:        ops.vertexCreate(id, easyjson.NewJSONObject()),
				invalidate: []string{id},
			}
		}
	case formTypeCreate:
		name := f.value("name")
		return func() tea.Msg {
			return mutationResultMsg{
				op: "type.create", target: name,
				res:        ops.typeCreate(name, easyjson.NewJSONObject()),
				invalidate: []string{name, NatsHubDomain + "/types"},
				refresh:    true,
			}
		}
	case formObjectCreate:
		id, tp := f.value("id"), f.value("type")
		return func() tea.Msg {
			return mutationResultMsg{
				op: "object.create", target: id,
				res:        ops.objectCreate(id, tp, easyjson.NewJSONObject()),
				invalidate: []string{id, tp, NatsHubDomain + "/objects"},
				refresh:    true,
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
