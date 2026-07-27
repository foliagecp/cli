package main

// Modes.
//
// There were six, and nothing said so. Three were booleans, one a nullable
// pointer, one the length of a slice — `len(m.queryResults) > 0` short-circuits
// updateNav into a handler where eight keys work and every other binding is
// silently dead — and one more pointer that is explicitly NOT a mode. The
// dispatch chain listed five of them.
//
// The consequence was five different meanings for Esc (close everything /
// cancel and wipe / destroy state / step back / cancel one thing then another)
// and four for ctrl+c, including search, where it fell through into the text
// input so the program could not be quit at all.
//
// The set is now named in one place and the exits are uniform:
//
//	Esc     pop one level, NEVER destroying data
//	ctrl+c  quit, from anywhere
//	q       quit while browsing; an ordinary character wherever text is typed
//
// The mode is derived rather than stored. A stored copy would be a second
// source of truth for something the existing fields already determine, and the
// first inconsistency between them would be a bug nobody could see.

type uiMode int

const (
	modeBrowse uiMode = iota
	modeHelp
	modeForm
	modeQuery
	modeSearch
	modeExport
	modeResults // a JPGQL result list is open
)

func (m tuiModel) mode() uiMode {
	switch {
	case m.helpOpen:
		return modeHelp
	case m.form != nil:
		return modeForm
	case m.queryMode:
		return modeQuery
	case m.searchMode:
		return modeSearch
	case m.exportMode:
		return modeExport
	case len(m.queryResults) > 0:
		return modeResults
	default:
		return modeBrowse
	}
}

func (k uiMode) String() string {
	switch k {
	case modeHelp:
		return "help"
	case modeForm:
		return "form"
	case modeQuery:
		return "query"
	case modeSearch:
		return "search"
	case modeExport:
		return "export"
	case modeResults:
		return "results"
	default:
		return "browse"
	}
}

// typesText reports whether the mode has a text field taking keystrokes. In
// those, `q` is a letter and quitting is ctrl+c — everywhere else `q` quits.
func (k uiMode) typesText() bool {
	switch k {
	case modeQuery, modeSearch, modeExport, modeForm:
		return true
	}
	return false
}
