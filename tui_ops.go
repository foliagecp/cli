package main

import (
	"fmt"

	"github.com/foliagecp/easyjson"
	sfMediators "github.com/foliagecp/sdk/statefun/mediator"
	sfp "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

// ── Operation result ──────────────────────────────────────────────────────────

type opStatus int

const (
	opApplied opStatus = iota // something actually changed
	opNoop                    // IDLE / empty op_stack — already in that state
	opFailed
)

// opResult is the UI-agnostic outcome of one mutation.
//
// It exists because the db client's write wrappers use the LENIENT error
// mapping — OpErrorFromOpMsg returns nil for BOTH ok and idle — so a nil error
// does not mean anything changed. Deleting a vertex that is already gone, or
// updating a body to the value it already holds, both come back as nil. A
// caller that reports "deleted" on every nil error is lying half the time.
//
// opNoop is not an error: it means the requested state was already satisfied.
type opResult struct {
	status  opStatus
	details string
	data    easyjson.JSON
	err     error
}

func (r opResult) ok() bool { return r.status != opFailed }

// resultFromErr maps a typed-wrapper call that only returns error. Without an
// op_stack or a raw status there is no way to tell applied from idle, so this
// reports opApplied and callers should phrase the message accordingly
// ("create sent" rather than "created"). Prefer rawOp or a *WithDetails twin.
func resultFromErr(err error) opResult {
	if err != nil {
		return opResult{status: opFailed, details: err.Error(), err: err}
	}
	return opResult{status: opApplied}
}

// resultFromDetails maps a CMDB *WithDetails call. Those request op_stack in
// the options, so an ABSENT or EMPTY op_stack is a stronger "nothing was
// written" signal than the status itself.
func resultFromDetails(data easyjson.JSON, err error) opResult {
	if err != nil {
		return opResult{status: opFailed, details: err.Error(), err: err}
	}
	// An ABSENT op_stack is not evidence of a no-op — it is the absence of
	// evidence. Only an op_stack that came back as an EMPTY ARRAY says the
	// server looked and wrote nothing.
	//
	// Conflating the two broke deletes on a runtime with a trash can: parking
	// an object replies without an op_stack, so a successful delete reported
	// `∅ already in that state` and the TUI refused to move off the vertex it
	// had just deleted — which is precisely how "I delete the object and it
	// stays on screen" happens.
	stack := data.GetByPath("op_stack")
	if stack.IsArray() && stack.ArraySize() == 0 {
		return opResult{status: opNoop, data: data}
	}
	return opResult{status: opApplied, data: data}
}

// rawOp issues a statefun request directly and reads om.Status, which is the
// only way to distinguish ok from idle for calls whose typed wrapper collapses
// both to nil. It is also the vehicle for payload fields the pinned client does
// not expose — notably the link-create "force" flag.
//
// Follows the existing raw-request precedent in gwalk.go (export/import).
func rawOp(typename, id string, payload *easyjson.JSON) opResult {
	if err := initDBClient(); err != nil {
		return opResult{status: opFailed, details: err.Error(), err: err}
	}
	om := sfMediators.OpMsgFromSfReply(
		dbClient.Request(sfp.AutoRequestSelect, typename, id, payload, nil),
	)
	switch om.Status {
	case sfMediators.SYNC_OP_STATUS_OK:
		return opResult{status: opApplied, data: om.Data}
	case sfMediators.SYNC_OP_STATUS_IDLE:
		return opResult{status: opNoop, details: om.Details, data: om.Data}
	default:
		return opResult{status: opFailed, details: om.Details, data: om.Data,
			err: fmt.Errorf("%s", om.Details)}
	}
}

// newOpPayload builds the payload every mutating CRUD call carries.
func newOpPayload() *easyjson.JSON {
	p := easyjson.NewJSONObject()
	p.SetByPath("op_time", easyjson.NewJSON(system.GetCurrentTimeNs()))
	return &p
}

// ── The operation layer ───────────────────────────────────────────────────────

// graphOps is the whole surface through which the CLI mutates the graph. Both
// the TUI and the non-interactive commands go through it, so behaviour cannot
// drift between them.
//
// It is a struct of functions rather than an interface on purpose: an interface
// with this many methods forces every test to supply a full fake. With a struct
// a test assigns only the field it exercises, and every other field stays nil —
// and a nil field panicking is CORRECT, it means the flow called something it
// had no business calling.
type graphOps struct {
	// low level — dbClient.Graph
	vertexCreate func(id string, body easyjson.JSON) opResult
	vertexUpdate func(id string, body easyjson.JSON, replace, upsert bool) opResult
	vertexDelete func(id string) opResult
	vertexRead   func(id string) (easyjson.JSON, error)

	linkCreate         func(from, to, name, tp string, tags []string, body easyjson.JSON, force bool) opResult
	linkUpdate         func(from, name string, tags []string, body easyjson.JSON, replace bool) opResult
	linkUpdateByToType func(from, to, tp string, tags []string, body easyjson.JSON, replace bool) opResult
	linkDelete         func(from, name string) opResult
	linkDeleteByToType func(from, to, tp string) opResult
	linkRead           func(from, name string) (easyjson.JSON, error)
	linkReadByToType   func(from, to, tp string) (easyjson.JSON, error)

	// high level — dbClient.CMDB
	typeCreate func(name string, body easyjson.JSON) opResult
	typeUpdate func(name string, body easyjson.JSON, replace, upsert bool) opResult
	typeDelete func(name string) opResult
	typeRead   func(name string) (easyjson.JSON, error)

	typesLinkCreate func(from, to, objectLinkType string, tags []string, body easyjson.JSON) opResult
	typesLinkUpdate func(from, to string, tags []string, body easyjson.JSON, replace bool) opResult
	typesLinkDelete func(from, to string) opResult
	typesLinkRead   func(from, to string) (easyjson.JSON, error)

	objectCreate func(id, originType string, body easyjson.JSON) opResult
	objectUpdate func(id string, body easyjson.JSON, replace bool, upsertType string) opResult
	objectDelete func(id string) opResult
	objectRead   func(id string) (easyjson.JSON, error)

	objectsLinkCreate func(from, to, name string, tags []string, body easyjson.JSON) opResult
	objectsLinkUpdate func(from, to string, tags []string, body easyjson.JSON, replace bool) opResult
	objectsLinkDelete func(from, to string) opResult
	objectsLinkRead   func(from, to string) (easyjson.JSON, error)

	subTypeSet    func(base, child string) opResult
	subTypeRemove func(base, child string) opResult

	superLinkCreate func(from, to, fromClaim, toClaim, name string, tags []string, body easyjson.JSON) opResult
	superLinkUpdate func(from, to, fromClaim, toClaim, name string, tags []string, body easyjson.JSON, replace bool) opResult
	superLinkDelete func(from, to, fromClaim, toClaim string) opResult
}

// ops is the live implementation. Tests swap it wholesale via withOps.
var ops = liveOps()

func liveOps() graphOps {
	// withClient runs fn only once the lazy dbClient singleton is up.
	withClient := func(fn func() opResult) opResult {
		if err := initDBClient(); err != nil {
			return opResult{status: opFailed, details: err.Error(), err: err}
		}
		return fn()
	}
	readWithClient := func(fn func() (easyjson.JSON, error)) (easyjson.JSON, error) {
		if err := initDBClient(); err != nil {
			return easyjson.NewJSONObject(), err
		}
		return fn()
	}

	return graphOps{
		// ── vertices ──────────────────────────────────────────────────────
		vertexCreate: func(id string, body easyjson.JSON) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.Graph.VertexCreate(id, body))
			})
		},
		vertexUpdate: func(id string, body easyjson.JSON, replace, upsert bool) opResult {
			return withClient(func() opResult {
				if upsert {
					return resultFromErr(dbClient.Graph.VertexUpdate(id, body, replace, true))
				}
				return resultFromErr(dbClient.Graph.VertexUpdate(id, body, replace))
			})
		},
		vertexDelete: func(id string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.Graph.VertexDelete(id))
			})
		},
		vertexRead: func(id string) (easyjson.JSON, error) {
			return readWithClient(func() (easyjson.JSON, error) {
				return dbClient.Graph.VertexRead(id, true)
			})
		},

		// ── raw links ─────────────────────────────────────────────────────
		//
		// force has no typed wrapper (the pinned client never sends it), so
		// the force path goes through rawOp with a hand-built payload.
		linkCreate: func(from, to, name, tp string, tags []string, body easyjson.JSON, force bool) opResult {
			if force {
				p := newOpPayload()
				p.SetByPath("to", easyjson.NewJSON(to))
				p.SetByPath("name", easyjson.NewJSON(name))
				p.SetByPath("type", easyjson.NewJSON(tp))
				p.SetByPath("body", body)
				if len(tags) > 0 {
					p.SetByPath("tags", easyjson.NewJSON(tags))
				}
				p.SetByPath("force", easyjson.NewJSON(true))
				// Bare `from` as the target id: the pinned client's own
				// seqFree() is a no-op (the "===<uid>" parallelism suffix is
				// commented out there), so every typed wrapper dispatches on
				// the bare id too. Matching that keeps the raw path
				// indistinguishable from the typed ones.
				return rawOp("functions.graph.api.link.create", from, p)
			}
			return withClient(func() opResult {
				return resultFromErr(dbClient.Graph.VerticesLinkCreate(from, to, name, tp, tags, body))
			})
		},
		linkUpdate: func(from, name string, tags []string, body easyjson.JSON, replace bool) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.Graph.VerticesLinkUpdate(from, name, tags, body, replace))
			})
		},
		linkUpdateByToType: func(from, to, tp string, tags []string, body easyjson.JSON, replace bool) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.Graph.VerticesLinkUpdateByToAndType(from, to, tp, tags, body, replace))
			})
		},
		linkDelete: func(from, name string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.Graph.VerticesLinkDelete(from, name))
			})
		},
		linkDeleteByToType: func(from, to, tp string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.Graph.VerticesLinkDeleteByToAndType(from, to, tp))
			})
		},
		linkRead: func(from, name string) (easyjson.JSON, error) {
			return readWithClient(func() (easyjson.JSON, error) {
				return dbClient.Graph.VerticesLinkRead(from, name, true)
			})
		},
		linkReadByToType: func(from, to, tp string) (easyjson.JSON, error) {
			return readWithClient(func() (easyjson.JSON, error) {
				return dbClient.Graph.VerticesLinkReadByToAndType(from, to, tp, true)
			})
		},

		// ── types ─────────────────────────────────────────────────────────
		typeCreate: func(name string, body easyjson.JSON) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.TypeCreate(name, body))
			})
		},
		typeUpdate: func(name string, body easyjson.JSON, replace, upsert bool) opResult {
			return withClient(func() opResult {
				if upsert {
					return resultFromErr(dbClient.CMDB.TypeUpdate(name, body, replace, true))
				}
				return resultFromErr(dbClient.CMDB.TypeUpdate(name, body, replace))
			})
		},
		typeDelete: func(name string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.TypeDelete(name))
			})
		},
		typeRead: func(name string) (easyjson.JSON, error) {
			return readWithClient(func() (easyjson.JSON, error) {
				return dbClient.CMDB.TypeRead(name)
			})
		},

		// ── types links ───────────────────────────────────────────────────
		typesLinkCreate: func(from, to, objectLinkType string, tags []string, body easyjson.JSON) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.TypesLinkCreate(from, to, objectLinkType, tags, body))
			})
		},
		typesLinkUpdate: func(from, to string, tags []string, body easyjson.JSON, replace bool) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.TypesLinkUpdate(from, to, tags, body, replace))
			})
		},
		typesLinkDelete: func(from, to string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.TypesLinkDelete(from, to))
			})
		},
		typesLinkRead: func(from, to string) (easyjson.JSON, error) {
			return readWithClient(func() (easyjson.JSON, error) {
				return dbClient.CMDB.TypesLinkRead(from, to)
			})
		},

		// ── objects ───────────────────────────────────────────────────────
		objectCreate: func(id, originType string, body easyjson.JSON) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.ObjectCreate(id, originType, body))
			})
		},
		objectUpdate: func(id string, body easyjson.JSON, replace bool, upsertType string) opResult {
			return withClient(func() opResult {
				if upsertType != "" {
					return resultFromDetails(dbClient.CMDB.ObjectUpdateWithDetails(id, body, replace, upsertType))
				}
				return resultFromDetails(dbClient.CMDB.ObjectUpdateWithDetails(id, body, replace))
			})
		},
		objectDelete: func(id string) opResult {
			return withClient(func() opResult {
				return resultFromDetails(dbClient.CMDB.ObjectDeleteWithDetails(id))
			})
		},
		objectRead: func(id string) (easyjson.JSON, error) {
			return readWithClient(func() (easyjson.JSON, error) {
				return dbClient.CMDB.ObjectRead(id)
			})
		},

		// ── objects links ─────────────────────────────────────────────────
		objectsLinkCreate: func(from, to, name string, tags []string, body easyjson.JSON) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.ObjectsLinkCreate(from, to, name, tags, body))
			})
		},
		objectsLinkUpdate: func(from, to string, tags []string, body easyjson.JSON, replace bool) opResult {
			return withClient(func() opResult {
				return resultFromDetails(dbClient.CMDB.ObjectsLinkUpdateWithDetails(from, to, tags, body, replace))
			})
		},
		objectsLinkDelete: func(from, to string) opResult {
			return withClient(func() opResult {
				return resultFromDetails(dbClient.CMDB.ObjectsLinkDeleteWithDetails(from, to))
			})
		},
		objectsLinkRead: func(from, to string) (easyjson.JSON, error) {
			return readWithClient(func() (easyjson.JSON, error) {
				return dbClient.CMDB.ObjectsLinkRead(from, to)
			})
		},

		// ── subtypes ──────────────────────────────────────────────────────
		subTypeSet: func(base, child string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.TypeSetSubType(base, child))
			})
		},
		subTypeRemove: func(base, child string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.TypeRemoveSubType(base, child))
			})
		},

		// ── supertype (claimed-type) object links ─────────────────────────
		superLinkCreate: func(from, to, fromClaim, toClaim, name string, tags []string, body easyjson.JSON) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.ObjectsLinkSuperTypeCreate(from, to, fromClaim, toClaim, name, tags, body))
			})
		},
		superLinkUpdate: func(from, to, fromClaim, toClaim, name string, tags []string, body easyjson.JSON, replace bool) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.ObjectsLinkSuperTypeUpdate(from, to, fromClaim, toClaim, name, tags, body, replace))
			})
		},
		superLinkDelete: func(from, to, fromClaim, toClaim string) opResult {
			return withClient(func() opResult {
				return resultFromErr(dbClient.CMDB.ObjectsLinkSuperTypeDelete(from, to, fromClaim, toClaim))
			})
		},
	}
}

// ── Mutation result message (TUI) ─────────────────────────────────────────────

// mutationResultMsg carries a completed mutation back into the TUI.
//
// It deliberately carries NO gen field. Every other async result in this TUI is
// dropped when its gen no longer matches the current load generation, because a
// stale READ is worthless. A mutation is the opposite: the side effect already
// happened on the server, so dropping the result would hide both the outcome
// and — worse — the cache invalidation, leaving stale entries behind. Same rule
// the export result already follows.
type mutationResultMsg struct {
	op          string // "vertex.create", "objects.link.delete", … — for the toast
	target      string // primary id the operation acted on
	res         opResult
	refresh     bool   // reload the current vertex afterwards
	navTo       string // non-empty ⇒ navigate here (post-delete escape)
	clearLinkIf string // clear the anchor when it equals this id
}
