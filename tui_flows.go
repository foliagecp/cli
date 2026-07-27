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
