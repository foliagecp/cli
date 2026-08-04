package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/urfave/cli/v2"
)

// Shared plumbing for the non-interactive CRUD commands.
//
// The commands are a thin shell over graphOps — the same layer the TUI uses —
// so behaviour cannot drift between the two front ends. Everything specific to
// running from a shell lives here: argument resolution, body/tag parsing,
// output formatting and exit codes.

// ── Argument normalisation ────────────────────────────────────────────────────

// reorderArgs moves flags ahead of positional arguments within the matched
// subcommand, so both of these work:
//
//	foliage-cli cmdb type create srv --body '{}'
//	foliage-cli cmdb type create --body '{}' srv
//
// Without it only the second form works. urfave/cli parses with the standard
// library's flag package, which stops parsing at the first non-flag argument —
// so in the first form "--body" and "{}" both become positional arguments and
// the command fails with a baffling "expected 1 argument". Since "verb subject
// --options" is how everyone actually types a CLI, silently mis-parsing it is
// not an option.
//
// The command path is walked against the real command tree, so only the tail
// belonging to the resolved subcommand is reordered, and whether a flag
// consumes the following token is taken from that subcommand's own flag
// definitions rather than guessed. Everything after a bare "--" is left alone.
func reorderArgs(argv []string, cmds []*cli.Command) []string {
	if len(argv) < 2 {
		return argv
	}

	// Walk the command path: consume tokens while they name a subcommand.
	prefix := []string{argv[0]}
	rest := argv[1:]
	var cur *cli.Command
	for len(rest) > 0 {
		next := matchCommand(cmds, rest[0])
		if next == nil {
			break
		}
		prefix = append(prefix, rest[0])
		rest = rest[1:]
		cur = next
		cmds = next.Subcommands
	}
	if cur == nil || len(rest) == 0 {
		return argv
	}

	takesValue := map[string]bool{}
	for _, f := range cur.Flags {
		_, isBool := f.(*cli.BoolFlag)
		for _, n := range f.Names() {
			takesValue[n] = !isBool
		}
	}

	var flags, positional []string
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		if tok == "--" {
			// Terminator: everything beyond is positional, verbatim.
			positional = append(positional, rest[i:]...)
			break
		}
		if len(tok) > 1 && strings.HasPrefix(tok, "-") {
			flags = append(flags, tok)
			name := strings.TrimLeft(tok, "-")
			// "--flag=value" already carries its value.
			if !strings.Contains(tok, "=") && takesValue[name] && i+1 < len(rest) {
				i++
				flags = append(flags, rest[i])
			}
			continue
		}
		positional = append(positional, tok)
	}

	out := make([]string, 0, len(argv))
	out = append(out, prefix...)
	out = append(out, flags...)
	out = append(out, positional...)
	return out
}

func matchCommand(cmds []*cli.Command, name string) *cli.Command {
	for _, c := range cmds {
		if c.Name == name {
			return c
		}
		for _, a := range c.Aliases {
			if a == name {
				return c
			}
		}
	}
	return nil
}

// ── Exit codes ────────────────────────────────────────────────────────────────
//
// 0  applied, or a no-op (the requested state already held)
// 1  the operation failed
// 2  bad arguments, or an operation that is refused outright
const (
	exitFailed  = 1
	exitRefused = 2
)

// usageErr produces a code-2 exit for bad input or a refused operation.
func usageErr(format string, a ...any) error {
	return cli.Exit(fmt.Sprintf("✗ "+format, a...), exitRefused)
}

// ── Built-in entities ─────────────────────────────────────────────────────────

// builtInVertices are the graph's structural roots. Deleting any of them breaks
// the CMDB topology in ways nothing recreates until the runtime restarts, so
// they are refused outright rather than merely confirmed. `trash_can` is on the
// list even though older runtimes do not have it — refusing to delete an id
// that does not exist costs nothing.
var builtInVertices = map[string]bool{
	"root": true, "types": true, "objects": true,
	"group": true, "nav": true, "trash_can": true,
}

// refuseBuiltIn returns an error when id names a structural vertex. The
// comparison is on the bare name so both `types` and `hub/types` are caught.
func refuseBuiltIn(id string) error {
	if builtInVertices[stripDomain(id)] {
		return usageErr("%s is a built-in vertex and cannot be deleted", id)
	}
	return nil
}

// ── Id resolution ─────────────────────────────────────────────────────────────

// validIDRe mirrors the server's link-name charset. Object and type ids are
// constrained by it too, because a create uses the id as a link name on the
// `objects`/`types` root. Note `.` is absent: it is the KV key separator.
var validIDRe = regexp.MustCompile(`\A[a-zA-Z0-9/_$#@%+=-]+\z`)

// validateID rejects ids the server would reject anyway, but with a message
// that says which rule was broken.
func validateID(kind, id string) error {
	if id == "" {
		return usageErr("%s id is empty", kind)
	}
	if strings.Count(id, "/") > 1 {
		return usageErr("%s id %q has more than one %q — only a single domain prefix is parsed", kind, id, "/")
	}
	if !validIDRe.MatchString(id) {
		return usageErr("%s id %q is invalid — allowed characters are a-z A-Z 0-9 / _ $ # @ %% + = - (dots are not allowed)", kind, id)
	}
	return nil
}

// cursorID returns the vertex the gwalk cursor currently points at.
func cursorID() (string, error) {
	if err := gWalkLoad(); err != nil {
		return "", err
	}
	id := gWalkData.GetByPath("id").AsStringDefault("")
	if id == "" {
		return "", usageErr("no id given and the gwalk cursor is not set — pass an id, or run: foliage-cli gwalk to <id>")
	}
	return id, nil
}

// resolveID takes the first positional argument, falling back to the gwalk
// cursor when it is absent. This mirrors `gwalk inspect` and `gwalk query`,
// which already operate on the cursor, so a session reads naturally:
//
//	foliage-cli gwalk to srv-1
//	foliage-cli cmdb object read
func resolveID(cCtx *cli.Context, kind string) (string, error) {
	id := cCtx.Args().First()
	if id == "" {
		var err error
		if id, err = cursorID(); err != nil {
			return "", err
		}
	}
	if err := validateID(kind, id); err != nil {
		return "", err
	}
	return id, nil
}

// requireArgs reads exactly n positional arguments, validating each as an id.
func requireArgs(cCtx *cli.Context, kinds ...string) ([]string, error) {
	if cCtx.NArg() != len(kinds) {
		return nil, usageErr("expected %d argument(s): <%s>", len(kinds), strings.Join(kinds, "> <"))
	}
	out := make([]string, len(kinds))
	for i, kind := range kinds {
		v := cCtx.Args().Get(i)
		if err := validateID(kind, v); err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// ── Body parsing ──────────────────────────────────────────────────────────────

// parseBody reads the --body flag, which accepts three sources, curl-style:
//
//	--body '{"a":1}'     inline JSON
//	--body @./body.json  a file
//	--body -             stdin
//
// An absent flag yields an empty object, which is what every create call sends
// when no body is supplied.
func parseBody(cCtx *cli.Context) (easyjson.JSON, error) {
	raw := cCtx.String("body")
	if raw == "" {
		return easyjson.NewJSONObject(), nil
	}

	var text string
	switch {
	case raw == "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return easyjson.NewJSONObject(), usageErr("cannot read body from stdin: %s", err)
		}
		text = string(b)
	case strings.HasPrefix(raw, "@"):
		b, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return easyjson.NewJSONObject(), usageErr("cannot read body file: %s", err)
		}
		text = string(b)
	default:
		text = raw
	}

	// Validate with encoding/json first: easyjson only reports a bool, while
	// this gives the offending position, which is what makes a typo fixable.
	var probe any
	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		return easyjson.NewJSONObject(), usageErr("body is not valid JSON: %s", err)
	}
	if _, isObj := probe.(map[string]any); !isObj {
		return easyjson.NewJSONObject(), usageErr("body must be a JSON object")
	}

	j, ok := easyjson.JSONFromString(text)
	if !ok {
		return easyjson.NewJSONObject(), usageErr("body could not be parsed")
	}
	return j, nil
}

// parseTags splits the --tags flag.
//
// An empty list is returned as nil, and every write wrapper drops nil/empty
// tags rather than sending them — so omitting --tags on an update leaves the
// existing tags alone. Clearing tags is only possible with --replace, and the
// update commands say so in their flag help.
func parseTags(cCtx *cli.Context) []string {
	raw := strings.TrimSpace(cCtx.String("tags"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ── Output ────────────────────────────────────────────────────────────────────

// reportOp prints the outcome of a mutation and maps it to an exit code.
//
// The distinction between applied and noop is deliberately visible: the client
// returns nil for both, and a caller that reports success either way is lying
// half the time.
func reportOp(cCtx *cli.Context, op, target string, r opResult) error {
	if cCtx.Bool("json") {
		out := easyjson.NewJSONObject()
		out.SetByPath("op", easyjson.NewJSON(op))
		out.SetByPath("target", easyjson.NewJSON(target))
		out.SetByPath("status", easyjson.NewJSON(opStatusName(r.status)))
		if r.details != "" {
			out.SetByPath("details", easyjson.NewJSON(r.details))
		}
		fmt.Println(out.ToString())
		if r.status == opFailed {
			return cli.Exit("", exitFailed)
		}
		return nil
	}

	switch r.status {
	case opApplied:
		fmt.Printf("✓ %s  %s\n", op, target)
	case opNoop:
		msg := r.details
		if msg == "" {
			msg = "already in that state"
		}
		fmt.Printf("∅ %s  %s — %s\n", op, target, msg)
	default:
		return cli.Exit(fmt.Sprintf("✗ %s  %s — %s", op, target, r.details), exitFailed)
	}
	return nil
}

func opStatusName(s opStatus) string {
	switch s {
	case opApplied:
		return "applied"
	case opNoop:
		return "noop"
	default:
		return "failed"
	}
}

// reportRead prints read output: the raw JSON with --json, pretty-printed
// otherwise.
func reportRead(cCtx *cli.Context, data easyjson.JSON, err error) error {
	if err != nil {
		return cli.Exit(fmt.Sprintf("✗ %s", err), exitFailed)
	}
	if cCtx.Bool("json") {
		fmt.Println(data.ToString())
		return nil
	}
	fmt.Println(JSONStrPrettyStringAnyway(&data, 0, 2))
	return nil
}

// ── Cascading-delete guard ────────────────────────────────────────────────────

// confirmCascade blocks a cascading delete unless --yes was given, and shows
// how much it would take with it. Scripts must opt in explicitly; there is no
// interactive prompt, because a script that hangs on a question is worse than
// one that fails loudly.
func confirmCascade(cCtx *cli.Context, what, target, consequence string) error {
	if cCtx.Bool("yes") {
		return nil
	}
	return usageErr("%s %s also %s.\n  Re-run with --yes to confirm.", what, target, consequence)
}

// typeObjectCount returns how many objects a type owns, for the cascade
// warning. A read failure is not fatal — the warning degrades to "an unknown
// number" rather than blocking the command.
func typeObjectCount(typeName string) (int, bool) {
	data, err := ops.typeRead(typeName)
	if err != nil {
		return 0, false
	}
	ids := data.GetByPath("object_ids")
	if !ids.IsArray() {
		return 0, false
	}
	return ids.ArraySize(), true
}

func objectCountPhrase(typeName string) string {
	if n, ok := typeObjectCount(typeName); ok {
		return fmt.Sprintf("removes %d object(s) of this type", n)
	}
	return "removes every object of this type"
}

// ── Shared flags ──────────────────────────────────────────────────────────────

func bodyFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "body",
		Usage: "JSON object: inline, @file, or - for stdin",
	}
}

func tagsFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "tags",
		Usage: "comma-separated tags; omitting this leaves existing tags untouched (clearing them needs --replace)",
	}
}

func replaceFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "replace",
		Usage: "replace the whole body instead of deep-merging it (merge can never remove a key or an array element)",
	}
}

func jsonFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "json",
		Usage: "machine-readable output",
	}
}

func yesFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "yes",
		Usage: "confirm a cascading delete",
	}
}
