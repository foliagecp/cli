package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foliagecp/easyjson"
	"github.com/urfave/cli/v2"
)

// ── reorderArgs ───────────────────────────────────────────────────────────────

// testCmds mirrors the real tree closely enough to exercise path walking.
func testCmds() []*cli.Command {
	return []*cli.Command{
		{
			Name: "cmdb",
			Subcommands: []*cli.Command{
				{
					Name: "type",
					Subcommands: []*cli.Command{
						{
							Name: "create",
							Flags: []cli.Flag{
								&cli.StringFlag{Name: "body"},
								&cli.BoolFlag{Name: "json"},
							},
						},
					},
				},
			},
		},
	}
}

func TestReorderArgs_HoistsFlagAfterPositional(t *testing.T) {
	in := []string{"foliage-cli", "cmdb", "type", "create", "srv", "--body", "{}"}
	want := []string{"foliage-cli", "cmdb", "type", "create", "--body", "{}", "srv"}

	got := reorderArgs(in, testCmds())
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("reorderArgs()\n got: %v\nwant: %v", got, want)
	}
}

func TestReorderArgs_AlreadyOrderedIsUnchanged(t *testing.T) {
	in := []string{"foliage-cli", "cmdb", "type", "create", "--body", "{}", "srv"}
	got := reorderArgs(in, testCmds())
	if strings.Join(got, " ") != strings.Join(in, " ") {
		t.Fatalf("reorderArgs() changed already-ordered args:\n got: %v\nwant: %v", got, in)
	}
}

func TestReorderArgs_BoolFlagDoesNotSwallowNextToken(t *testing.T) {
	// --json takes no value, so "srv" must stay a positional argument.
	in := []string{"foliage-cli", "cmdb", "type", "create", "--json", "srv"}
	got := reorderArgs(in, testCmds())
	if got[len(got)-1] != "srv" {
		t.Fatalf("bool flag swallowed the positional: %v", got)
	}
}

func TestReorderArgs_EqualsFormKeepsItsValue(t *testing.T) {
	in := []string{"foliage-cli", "cmdb", "type", "create", "srv", "--body={}"}
	got := reorderArgs(in, testCmds())
	want := []string{"foliage-cli", "cmdb", "type", "create", "--body={}", "srv"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("reorderArgs()\n got: %v\nwant: %v", got, want)
	}
}

func TestReorderArgs_DoubleDashIsRespected(t *testing.T) {
	in := []string{"foliage-cli", "cmdb", "type", "create", "--", "--body", "x"}
	got := reorderArgs(in, testCmds())
	// Everything past "--" must survive verbatim and stay at the end.
	joined := strings.Join(got, " ")
	if !strings.HasSuffix(joined, "-- --body x") {
		t.Fatalf("args after -- were rewritten: %v", got)
	}
}

func TestReorderArgs_UnknownCommandLeftAlone(t *testing.T) {
	in := []string{"foliage-cli", "nope", "srv", "--body", "{}"}
	got := reorderArgs(in, testCmds())
	if strings.Join(got, " ") != strings.Join(in, " ") {
		t.Fatalf("unknown command path should be untouched: %v", got)
	}
}

func TestReorderArgs_ShortArgvIsSafe(t *testing.T) {
	for _, in := range [][]string{nil, {"foliage-cli"}} {
		if got := reorderArgs(in, testCmds()); len(got) != len(in) {
			t.Fatalf("reorderArgs(%v) = %v, want unchanged", in, got)
		}
	}
}

// ── id validation ─────────────────────────────────────────────────────────────

func TestValidateID(t *testing.T) {
	valid := []string{"srv", "srv-1", "hub/srv", "a_b", "x$y#z%w+q=r"}
	for _, id := range valid {
		if err := validateID("vertex", id); err != nil {
			t.Errorf("validateID(%q) = %v, want nil", id, err)
		}
	}

	invalid := map[string]string{
		"":          "empty",
		"a.b":       "dot is the KV separator",
		"a/b/c":     "more than one domain separator",
		"has space": "space is outside the charset",
	}
	for id, why := range invalid {
		if err := validateID("vertex", id); err == nil {
			t.Errorf("validateID(%q) = nil, want an error (%s)", id, why)
		}
	}
}

// ── built-in guard ────────────────────────────────────────────────────────────

func TestRefuseBuiltIn(t *testing.T) {
	for _, id := range []string{"root", "types", "objects", "trash_can", "hub/types", "hub/root"} {
		if err := refuseBuiltIn(id); err == nil {
			t.Errorf("refuseBuiltIn(%q) = nil, want a refusal", id)
		}
	}
	for _, id := range []string{"srv", "hub/srv", "rootish"} {
		if err := refuseBuiltIn(id); err != nil {
			t.Errorf("refuseBuiltIn(%q) = %v, want nil", id, err)
		}
	}
}

// ── flag parsing ──────────────────────────────────────────────────────────────

// ctxWith builds a cli.Context carrying the given flag values.
func ctxWith(t *testing.T, flags map[string]string, bools map[string]bool) *cli.Context {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	for k, v := range flags {
		fs.String(k, v, "")
	}
	for k, v := range bools {
		fs.Bool(k, v, "")
	}
	return cli.NewContext(cli.NewApp(), fs, nil)
}

func TestParseTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ", []string{"a", "b"}},
		{"a,a,b", []string{"a", "b"}}, // deduped
		{",,", nil},
	}
	for _, c := range cases {
		got := parseTags(ctxWith(t, map[string]string{"tags": c.in}, nil))
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("parseTags(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseBody_AbsentIsEmptyObject(t *testing.T) {
	b, err := parseBody(ctxWith(t, map[string]string{"body": ""}, nil))
	if err != nil {
		t.Fatalf("parseBody() = %v, want nil", err)
	}
	if !b.IsObject() {
		t.Errorf("body = %s, want an empty object", b.ToString())
	}
}

func TestParseBody_Inline(t *testing.T) {
	b, err := parseBody(ctxWith(t, map[string]string{"body": `{"a":1}`}, nil))
	if err != nil {
		t.Fatalf("parseBody() = %v", err)
	}
	if got := b.GetByPath("a").AsNumericDefault(0); got != 1 {
		t.Errorf("body.a = %v, want 1", got)
	}
}

func TestParseBody_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "body.json")
	if err := os.WriteFile(path, []byte(`{"k":"v"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := parseBody(ctxWith(t, map[string]string{"body": "@" + path}, nil))
	if err != nil {
		t.Fatalf("parseBody() = %v", err)
	}
	if got := b.GetByPath("k").AsStringDefault(""); got != "v" {
		t.Errorf("body.k = %q, want %q", got, "v")
	}
}

func TestParseBody_RejectsMalformedAndNonObject(t *testing.T) {
	if _, err := parseBody(ctxWith(t, map[string]string{"body": "not json"}, nil)); err == nil {
		t.Error("malformed JSON should be rejected")
	}
	if _, err := parseBody(ctxWith(t, map[string]string{"body": "[1,2]"}, nil)); err == nil {
		t.Error("a non-object body should be rejected")
	}
}

func TestParseBody_MissingFileIsAnError(t *testing.T) {
	if _, err := parseBody(ctxWith(t, map[string]string{"body": "@/nonexistent/x.json"}, nil)); err == nil {
		t.Error("a missing body file should be reported")
	}
}

// ── command wiring (no network) ───────────────────────────────────────────────

// runCLI drives the real command tree through the real argument normalisation.
//
// ExitErrHandler is overridden to a no-op: by default urfave/cli hands an
// ExitCoder to HandleExitCoder, which calls os.Exit and would take the test
// binary down with it. Suppressing it lets the error be returned and asserted.
func runCLI(args ...string) error {
	app := &cli.App{
		Commands:       []*cli.Command{graphCommand(), cmdbCommand()},
		ExitErrHandler: func(*cli.Context, error) {},
	}
	argv := append([]string{"foliage-cli"}, args...)
	return app.Run(reorderArgs(argv, app.Commands))
}

func TestCmd_TypeCreate_PassesIDAndBody(t *testing.T) {
	var gotName string
	var gotBody easyjson.JSON
	withOps(t, graphOps{
		typeCreate: func(name string, body easyjson.JSON) opResult {
			gotName, gotBody = name, body
			return opResult{status: opApplied}
		},
	})

	if err := runCLI("cmdb", "type", "create", "srv", "--body", `{"a":1}`); err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if gotName != "srv" {
		t.Errorf("type name = %q, want %q", gotName, "srv")
	}
	if got := gotBody.GetByPath("a").AsNumericDefault(0); got != 1 {
		t.Errorf("body.a = %v, want 1", got)
	}
}

func TestCmd_LinkCreate_DefaultsNameToTarget(t *testing.T) {
	// The server names a link after its target when no name is given; the CLI
	// mirrors that so the common case needs no --name.
	var gotName string
	withOps(t, graphOps{
		linkCreate: func(from, to, name, tp string, tags []string, body easyjson.JSON, force bool) opResult {
			gotName = name
			return opResult{status: opApplied}
		},
	})

	if err := runCLI("graph", "link", "create", "hub/a", "--to", "hub/b", "--type", "rel"); err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if gotName != "b" {
		t.Errorf("link name = %q, want %q (target id without its domain)", gotName, "b")
	}
}

func TestCmd_LinkCreate_PassesTagsAndForce(t *testing.T) {
	var gotTags []string
	var gotForce bool
	withOps(t, graphOps{
		linkCreate: func(from, to, name, tp string, tags []string, body easyjson.JSON, force bool) opResult {
			gotTags, gotForce = tags, force
			return opResult{status: opApplied}
		},
	})

	err := runCLI("graph", "link", "create", "hub/a",
		"--to", "hub/b", "--type", "rel", "--tags", "x,y", "--force")
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if strings.Join(gotTags, ",") != "x,y" {
		t.Errorf("tags = %v, want [x y]", gotTags)
	}
	if !gotForce {
		t.Error("--force did not reach the operation layer")
	}
}

func TestCmd_LinkDelete_RequiresASelector(t *testing.T) {
	withOps(t, graphOps{}) // any call would panic on a nil field
	err := runCLI("graph", "link", "delete", "hub/a")
	if err == nil {
		t.Fatal("deleting a link without --name or --to/--type should fail")
	}
	if ec, ok := err.(cli.ExitCoder); !ok || ec.ExitCode() != exitRefused {
		t.Errorf("exit code = %v, want %d", err, exitRefused)
	}
}

func TestCmd_TypeDelete_CascadeNeedsYes(t *testing.T) {
	called := false
	withOps(t, graphOps{
		typeDelete: func(name string) opResult { called = true; return opResult{status: opApplied} },
		typeRead: func(name string) (easyjson.JSON, error) {
			ids := easyjson.NewJSONArray()
			ids.AddToArray(easyjson.NewJSON("hub/srv-1"))
			ids.AddToArray(easyjson.NewJSON("hub/srv-2"))
			return easyjson.NewJSONObjectWithKeyValue("object_ids", ids), nil
		},
	})

	err := runCLI("cmdb", "type", "delete", "srv")
	if err == nil {
		t.Fatal("a cascading delete without --yes should be refused")
	}
	if called {
		t.Error("typeDelete must not run without --yes")
	}
	if !strings.Contains(err.Error(), "2 object(s)") {
		t.Errorf("refusal should state the blast radius, got: %v", err)
	}

	if err := runCLI("cmdb", "type", "delete", "srv", "--yes"); err != nil {
		t.Fatalf("run with --yes = %v, want nil", err)
	}
	if !called {
		t.Error("typeDelete should run once --yes is given")
	}
}

func TestCmd_ObjectDelete_RefusesBuiltIn(t *testing.T) {
	withOps(t, graphOps{}) // a call would panic
	err := runCLI("cmdb", "object", "delete", "objects")
	if err == nil {
		t.Fatal("deleting a built-in vertex should be refused")
	}
	if ec, ok := err.(cli.ExitCoder); !ok || ec.ExitCode() != exitRefused {
		t.Errorf("exit code = %v, want %d", err, exitRefused)
	}
}

func TestCmd_FailedOpExitsOne(t *testing.T) {
	withOps(t, graphOps{
		vertexCreate: func(id string, body easyjson.JSON) opResult {
			return opResult{status: opFailed, details: "boom"}
		},
	})
	err := runCLI("graph", "vertex", "create", "x")
	if err == nil {
		t.Fatal("a failed operation should produce an error")
	}
	if ec, ok := err.(cli.ExitCoder); !ok || ec.ExitCode() != exitFailed {
		t.Errorf("exit code = %v, want %d", err, exitFailed)
	}
}

func TestCmd_NoopExitsZero(t *testing.T) {
	// IDLE is not a failure: the requested state already held.
	withOps(t, graphOps{
		vertexDelete: func(id string) opResult { return opResult{status: opNoop, details: "does not exist"} },
	})
	if err := runCLI("graph", "vertex", "delete", "x"); err != nil {
		t.Fatalf("a no-op should exit 0, got %v", err)
	}
}

func TestCmd_SubtypeAdd_PassesBothTypes(t *testing.T) {
	var base, child string
	withOps(t, graphOps{
		subTypeSet: func(b, c string) opResult { base, child = b, c; return opResult{status: opApplied} },
	})
	if err := runCLI("cmdb", "type", "subtype", "add", "hw", "server"); err != nil {
		t.Fatalf("run = %v", err)
	}
	if base != "hw" || child != "server" {
		t.Errorf("subTypeSet(%q,%q), want (hw,server)", base, child)
	}
}
