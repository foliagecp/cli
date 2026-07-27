package main

import (
	"fmt"

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
