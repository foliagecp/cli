package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Discoverability.
//
// A key that is neither in the status bar nor behind `?` may as well not
// exist — which is how i/d/D/x first shipped, and why the user could not
// check anything. The keymap table is now the one source both are rendered
// from, so those two cannot disagree. What is left to prove is that the table
// agrees with the code, in both directions.

// TestKeymap_MatchesTheBrowseHandlers reads the actual `case` labels out of
// updateNav and compares them with the table.
//
// Parsing the source is not elegant, and it is the only check that can catch
// what actually went wrong twice: a key added to the switch and to nothing
// else, and a key removed from the switch while its documentation stayed. A
// hand-maintained list of "keys that exist" would be a third copy of the same
// fact and would rot the same way.
func TestKeymap_MatchesTheBrowseHandlers(t *testing.T) {
	bound := browseKeysFromSource(t)
	documented := map[string]bool{}
	for _, k := range browseKeys() {
		documented[normKey(k)] = true
	}

	var undocumented []string
	for k := range bound {
		if !documented[normKey(k)] {
			undocumented = append(undocumented, k)
		}
	}
	sort.Strings(undocumented)
	if len(undocumented) > 0 {
		t.Errorf("bound in updateNav but absent from the keymap: %v\n"+
			"a key nobody can discover may as well not exist", undocumented)
	}

	var unbound []string
	for _, k := range browseKeys() {
		if aliasOnly[normKey(k)] {
			continue
		}
		if !bound[normKey(k)] {
			unbound = append(unbound, k)
		}
	}
	sort.Strings(unbound)
	if len(unbound) > 0 {
		t.Errorf("in the keymap but bound to nothing: %v\n"+
			"documentation that lies is worse than none — the user presses it, "+
			"nothing happens, and now they distrust the rest of the screen", unbound)
	}
}

// aliasOnly are tokens handled outside updateNav's own switch, so their
// absence from it is not a missing binding.
var aliasOnly = map[string]bool{
	"ctrl+c": true, // handled centrally, above the dispatch
}

// normKey maps what the table PRINTS to what bubbletea REPORTS. The table
// shows arrow glyphs because that is what is on the keycap.
func normKey(k string) string {
	switch k {
	case "↑":
		return "up"
	case "↓":
		return "down"
	case "←":
		return "left"
	case "→":
		return "right"
	}
	return strings.ToLower(strings.TrimSpace(k))
}

// browseKeysFromSource extracts the string literals from `case "…":` labels
// inside updateNav.
func browseKeysFromSource(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("tui_update.go")
	if err != nil {
		t.Fatalf("reading tui_update.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func (m tuiModel) updateNav(")
	if start < 0 {
		t.Fatal("updateNav not found — this test needs updating alongside the rename")
	}
	end := strings.Index(body[start+10:], "\nfunc ")
	if end < 0 {
		t.Fatal("could not find the end of updateNav")
	}
	fn := body[start : start+10+end]

	out := map[string]bool{}
	caseRe := regexp.MustCompile(`(?m)^\s*case ("(?:[^"]+)"(?:,\s*"[^"]+")*):`)
	litRe := regexp.MustCompile(`"([^"]+)"`)
	for _, mt := range caseRe.FindAllStringSubmatch(fn, -1) {
		for _, lit := range litRe.FindAllStringSubmatch(mt[1], -1) {
			out[normKey(lit[1])] = true
		}
	}
	if len(out) < 10 {
		t.Fatalf("only found %d cases in updateNav — the parse is wrong", len(out))
	}
	return out
}

func TestKeymap_EveryGroupIsRendered(t *testing.T) {
	listed := map[string]bool{}
	for _, g := range helpGroups {
		listed[g] = true
	}
	for _, b := range keymap {
		if !listed[b.group] {
			t.Errorf("binding %q is in group %q, which helpGroups does not render", b.keys, b.group)
		}
	}
}

func TestKeymap_StatusHintsAreAllRealBindings(t *testing.T) {
	// The status bar is the only keymap most users read. A hint naming a key
	// that does nothing is the worst version of the drift this table exists
	// to prevent.
	for _, b := range keymap {
		if b.hint == "" {
			continue
		}
		k, _, ok := strings.Cut(b.hint, ":")
		if !ok {
			t.Errorf("hint %q should be key:action", b.hint)
			continue
		}
		// "jk" is the compact spelling of the "j k  ↑ ↓" row; check every
		// character of the hint key is one of the binding's.
		for _, r := range k {
			if !strings.ContainsRune(b.keys, r) {
				t.Errorf("hint %q names %q, which is not among its binding's keys (%q)",
					b.hint, string(r), b.keys)
			}
		}
	}
}

func TestHelp_ShowsEveryBinding(t *testing.T) {
	// Both surfaces come from the keymap, so this is nearly a tautology — which is
	// the point. It used to be two hand-written lists that drifted.
	m := makeModel("hub/x", nil, nil)
	m.width, m.height = 200, 60
	out := stripANSI(m.renderHelp())
	for _, b := range keymap {
		if !strings.Contains(out, b.desc) {
			t.Errorf("the help screen omits %q (%s)", b.keys, b.desc)
		}
	}
}

func TestHelp_OpensAndAnyKeyCloses(t *testing.T) {
	m := makeModel("root", nil, nil)

	m = update(m, key("?"))
	if !m.helpOpen {
		t.Fatal("? should open the help screen")
	}
	if !strings.Contains(m.View(), "Keys") {
		t.Error("the help screen should be what View renders while it is open")
	}

	m = update(m, key("j"))
	if m.helpOpen {
		t.Error("any key should close the help screen")
	}
}

func TestHelp_DoesNotActOnTheKeyThatClosesIt(t *testing.T) {
	// Closing must swallow the keystroke: pressing D to dismiss help should not
	// then stage a delete.
	m := makeModel("hub/x", nil, nil)
	m = update(m, key("?"))
	m = update(m, key("D"))

	if m.helpOpen {
		t.Fatal("D should have closed the help screen")
	}
	if m.form != nil {
		t.Error("the key that closed the help screen must not also act")
	}
}

func TestHelp_CoversEveryEditorChord(t *testing.T) {
	var listed strings.Builder
	for _, b := range keymap {
		listed.WriteString(b.keys + " ")
	}
	for _, chord := range []string{"ctrl+s", "ctrl+r", "ctrl+e", "ctrl+t"} {
		if !strings.Contains(listed.String(), chord) {
			t.Errorf("%s is bound inside the body editor but not documented", chord)
		}
	}
}

func TestHelp_RendersInANarrowTerminal(t *testing.T) {
	m := makeModel("root", nil, nil)
	m.width = 60
	m.height = 30
	m.helpOpen = true

	out := m.renderHelp()
	if out == "" {
		t.Fatal("help should render at 60 columns")
	}
	for _, line := range strings.Split(out, "\n") {
		if w := len([]rune(stripANSI(line))); w > m.width+2 {
			t.Errorf("help line overflows a %d-column terminal (%d cells): %q", m.width, w, line)
		}
	}
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// ── Uniform exits ─────────────────────────────────────────────────────────────

// TestModes_CtrlCQuitsFromEverywhere. It used to quit in two modes, close in
// three, and in search fall through into the text input — so the one key every
// terminal program shares could not be relied on to end this one.
func TestModes_CtrlCQuitsFromEverywhere(t *testing.T) {
	ctrlC := tea.KeyMsg{Type: tea.KeyCtrlC}
	fvi := makeVertexInfo("hub/x", nil, nil)

	setups := map[string]func() tuiModel{
		"browse": func() tuiModel { return makeModel("hub/x", nil, &fvi) },
		"help":   func() tuiModel { return update(makeModel("hub/x", nil, &fvi), key("?")) },
		"query":  func() tuiModel { return update(makeModel("hub/x", nil, &fvi), key("/")) },
		"search": func() tuiModel { return update(makeModel("hub/x", nil, &fvi), key("f")) },
		"export": func() tuiModel { return update(makeModel("hub/x", nil, &fvi), key("e")) },
		"form":   func() tuiModel { return update(makeModel("hub/x", nil, &fvi), key("n")) },
		"results": func() tuiModel {
			m := makeModel("hub/x", nil, &fvi)
			m.queryResults = []string{"hub/a"}
			return m
		},
	}
	for name, setup := range setups {
		_, cmd := setup().Update(ctrlC)
		if cmd == nil {
			t.Errorf("%s: ctrl+c did nothing — it must quit from every mode", name)
			continue
		}
		if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
			t.Errorf("%s: ctrl+c did not quit", name)
		}
	}
}

// TestModes_EscNeverDestroysData. Esc had five meanings; the one it could not
// express was "leave things as you found them". In search it wiped the filter
// outright, even though `f` deliberately pre-seeds the input with it.
func TestModes_EscRestoresTheFilterItWasEditing(t *testing.T) {
	m := makeModel("hub/x", threeLinks(), nil)
	m = m.applySearch("l1")

	m = update(m, key("f")) // edit the filter
	m = typeText(m, "xyz")  // …type something else
	m = update(m, keyEsc()) // …and change your mind

	if m.searchQuery != "l1" {
		t.Errorf("searchQuery = %q after Esc, want the filter that was in force (l1)", m.searchQuery)
	}
}

func TestModes_EveryModeIsNamedInOnePlace(t *testing.T) {
	// The results list used to be a mode nothing declared — updateNav
	// short-circuited on len(queryResults) > 0, so eight keys worked and every
	// other binding was silently dead with no way to tell.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", nil, &fvi)
	if got := m.mode(); got != modeBrowse {
		t.Errorf("mode = %v, want browse", got)
	}
	m.queryResults = []string{"hub/a"}
	if got := m.mode(); got != modeResults {
		t.Errorf("with results open mode = %v, want results", got)
	}
}
