# Foliage CLI

A command-line interface for navigating and managing a [Foliage](https://github.com/foliagecp/sdk) graph over NATS.

## Configuration

| Environment variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://nats:foliage@nats:4222` | NATS server URL |
| `NATS_HUB_DOMAIN` | `hub` | Hub domain name |
| `NATS_REQUEST_TIMEOUT_SEC` | `60` | Request timeout in seconds |
| `FOLIAGE_CLI_DIR` | `~/.foliage-cli` | Directory for persisting cursor state |

## Commands

### `tui` — interactive graph browser

```
foliage-cli tui
```

Opens a full-screen three-panel TUI. Navigation starts from the last persisted
position (or `root` on first run). Below 90 columns it falls back to a single
column, which renders the same things — including every modal.

Here the outgoing column has focus with a link selected, so the centre panel is
showing that link. Standing on the centre column instead shows the vertex, and
neither side panel is highlighted:

```
┌──────────────────────────────────────────────────────────────────┐
│  ◈ hub/router-1                                                  │
├──────────────────┬───────────────────────────┬────────────────────┤
│ ← INCOMING (2)  │ ⎯ port-b   [objects-link]  │ → OUTGOING (5)    │
│                  │                            │                    │
│  managed_by  1   │  router-1  ──▶  port-b     │  contains     3   │
│    admin ←       │                            │    port-a →       │
│                  │  name  port-b              │  ► port-b →       │
│                  │  type  contains            │    port-c →       │
│                  │  via   objects-link        │                    │
│                  │  tags  uplink, prod        │  depends_on   2   │
│                  │                            │    upstream →     │
│                  │  body                      │    gateway →      │
│                  │    speed:  10G             │                    │
└──────────────────┴───────────────────────────┴────────────────────┘
  jk:nav  Enter:go  b:back  n:new  i:edit  d:del  L:link  ?:help
```

Press `?` inside the TUI for the full keymap — the status bar only has room for
the most-used bindings. Both are rendered from one table in the source, so they
cannot disagree with each other, and a test checks that table against the key
handlers, so neither can disagree with the code.

### You are always standing on a subject

The subject is either a vertex or a link, and which one is decided by the
column you are standing in: the centre column is the vertex, a side column is
the link under its cursor. **The centre panel always shows the subject in
full** — for a link that means its endpoints, name, type, API tier, tags and
body, none of which used to be visible anywhere.

Every action applies to the subject:

| Key | Action |
|---|---|
| `i` | edit the subject |
| `d` | delete the subject |
| `y` | yank the subject's body, to paste with `ctrl+t` |

One rule, instead of the four this replaced. There is deliberately no key for
"the vertex regardless of where I am standing": the column is the selector, and
a second way to say it would be a second answer to the question the columns
exist to answer. In a side column with no link under the cursor there is simply
no subject, and the keys say so rather than quietly acting on the vertex.

It is also why editing a link is not a binding you have to be told about: you
can see the link, so pressing the edit key over it is the obvious move.

**Navigate and view**

| Key | Action |
|---|---|
| `h` / `l` | Move between the columns: incoming · the vertex · outgoing |
| `j` / `↓`, `k` / `↑` | Move cursor |
| `Enter` | Navigate to selected vertex; on a group header — toggle collapse |
| `Tab` | Toggle collapse of the current link-type group |
| `b` / `Backspace` | Go back (history stack) |
| `v` | Toggle raw JSON body vs. key-value view |
| `c` | Copy current vertex ID to clipboard |
| `:` | Go to a vertex by id · `Tab` cycles the built-in ones |
| `/` | JPGQL query from current vertex |
| `f` | Live filter links by name · `Esc` restores the previous filter |
| `e` | Export graph to file (graphml / dot / json2xml), choose depth |
| `r` / `Ctrl+R` | Refresh current vertex · also clear the entire cache |
| `R` | Jump to `root` and reset all state, including a pending link |
| `g` / `G` | Scroll the body up / down |
| `x` | Switch the CRUD API between high-level and low-level |
| `?` | Show the full keymap |
| `q` / `Ctrl+C` | Quit |

`Esc` always pops one level and never destroys data. `Ctrl+C` quits from
anywhere. `q` quits while browsing and is an ordinary letter wherever there is
a text field. Wherever `Tab` cycles something, the status bar says so.

**The centre column is part of the focus cycle**, and that is what decides the
subject. Standing on it means you are working with the vertex, and neither side
panel is highlighted — an unfocused panel never highlights anything, so nothing
on screen claims a link is selected when none is. `h`/`l` step one column at a
time and clamp at the ends; step into a link panel and its first row is
highlighted straight away, because there the claim is true.

**The CRUD API is always stated** at the left of the status bar: `CRUD:
high-level` or `CRUD: low-level`, switched with `x`. It governs create, edit
and delete — not navigation, which always walks raw edges.

It governs them strictly. The high-level API knows types, objects, types-links
and objects-links; the low-level one knows raw vertices and raw links. An
operation with no meaning under the armed API is refused, and the refusal names
the key that makes it possible:

```
x is a plain vertex — the high-level API only knows types and objects
                                       — press x to switch the CRUD API
```

The create menu obeys the same rule, so `type` is unavailable in low-level mode
and `raw vertex` in high-level mode, each saying why. And confirmations name
what they are about to remove — `DELETE TYPE srv`, `Delete OBJECT srv-1`,
`Delete VERTEX srv with the LOW-LEVEL API` — because the same vertex is very
different amounts of graph through the two APIs.

The header badge names what you are standing on — `[type]`, `[object of srv]`,
`[built-in]`, `[vertex]`, or `[object · instance-of link missing]` for a
half-written one. It is worth reading, because it is what the create menu is
gated on.

### Create

| Key | Action |
|---|---|
| `n` | New… — what can be created from where you are standing |
| `L` | Start a link here · press again at the target to commit |
| `Esc` | Cancel a pending link |

**You create a thing where that thing lives.** Types are created from
`hub/types`, objects from their own type, sub-types from the parent type, and a
raw vertex is always attached to the vertex you create it from. This is not
ceremony: the TUI finds things by walking the graph, so anything created
outside its home would be unreachable by the very tool that made it. After a
successful create you are moved onto the new entity.

Where an action does not belong, the menu still lists it, says where it does
live, and **pressing it takes you there** — so the rule is learned by using it
rather than by being refused.

Linking is a two-step commit, because typing a target id defeats the point of
having a browser:

1. Stand on the source, press `L` — a banner appears: `◆ LINK PENDING  srv-1 ──▶ …`
2. Navigate anywhere. Every key keeps working; the banner stays.
3. At the target press `L` again to commit, or `Esc` to cancel.

Which API the link goes through is derived from the endpoints and shown in the
form title: type→type creates a **types-link** (a schema declaration),
object→object an **objects-link**, and anything else a **raw link**. Inside the
form `←`/`→` swaps the direction.

An objects-link form also offers **from as** / **to as**, prefilled with the
endpoints' real types. Naming a super-type of either instead links the two
objects under a schema declared further up the hierarchy. Leave them alone for
the ordinary case.

### Modify and delete

`i` (or `t`, which lands on the tags row) opens the editor for the selected
link: its tags and its body in one form, because the server applies `replace`
to both with a single flag. The form is built from a read of the link, never
from the list view — the list deliberately does not fetch tags or bodies, and a
form seeded from it would show blanks over real data.

Inside the body editor: `Ctrl+S` apply · `Ctrl+R` switch MERGE ⇄ REPLACE ·
`Ctrl+E` open `$EDITOR` · `Ctrl+T` paste the yanked body · `Esc` cancel. JSON is
validated as you type and submission is blocked while it is invalid.
Machine-owned paths (`triggers`, `cache`, a types-link's `type`, …) are held
aside and listed under the editor, so nothing is lost.

Deletion routes through whichever API owns the edge, which changes what it
does:

- a **types-link** deletes the schema declaration *and* the corresponding link
  on every object of the source type — so it asks you to type the type's name,
  the same as deleting a type
- a **sub-type** edge removes the inheritance relation and re-runs the
  computation on every descendant — likewise confirmed by name
- everything else is a single-key `y`/`n`

Deleting a **type** requires typing its name, because it removes every object
of that type. Structural vertices (`root`, `types`, `objects`, `trash_can`, …)
are refused outright.

After a delete you land on the deleted entity's **home** — an object goes up to
its type, a type to `hub/types` — which is the mirror of the rule that governs
creation. That is also where the deletion is visible: the entity is no longer
in the list. Staying put would show its body as though nothing had happened,
and on a runtime with a trash can that is literally true, because a deleted
object is parked rather than erased.

Such a runtime re-links the object under `hub/trash_can` with its original type
and the deletion moment on the edge — so select that link in the trash can and
the centre panel shows both.

Applied and no-op are reported distinctly (`✓` vs `∅`): the client returns
success for both, and a delete that deleted nothing leaves you where you are.

---

### `graph` and `cmdb` — CRUD from the shell

Everything the TUI can do is also available non-interactively, for scripting
and for building reproducible fixtures. The two groups mirror the SDK's own
split: `graph` is the low-level API (raw vertices and links, no CMDB
semantics), `cmdb` is the typed one.

```
graph  vertex      create|update|delete|read
graph  link        create|update|delete|read
cmdb   type        create|update|delete|read
cmdb   type subtype add|rm
cmdb   typeslink   create|update|delete|read
cmdb   object      create|update|delete|read
cmdb   objectslink create|update|delete|read
cmdb   objectslink super create|update|delete
```

A positional id may be omitted, in which case the `gwalk` cursor is used —
the same idiom `gwalk inspect` and `gwalk query` already follow:

```sh
foliage-cli gwalk to srv-1
foliage-cli graph link create --to rack-A --type rel   # from = srv-1
foliage-cli cmdb object read                           # the cursor vertex
```

A worked example — schema first, then instances:

```sh
foliage-cli cmdb type create srv --body '{"desc":"server"}'
foliage-cli cmdb type create rack
foliage-cli cmdb typeslink create srv rack --object-link-type mounted_in
foliage-cli cmdb object create srv-1 --type srv --body @srv-1.json
foliage-cli cmdb object create rack-A --type rack
foliage-cli cmdb objectslink create srv-1 rack-A
```

**Shared flags**

| Flag | Meaning |
|---|---|
| `--body` | inline JSON, `@file`, or `-` for stdin |
| `--tags a,b` | comma separated. Omitting it leaves existing tags alone; **clearing them needs `--replace`**, because empty tags are never sent |
| `--replace` | replace the body instead of deep-merging it (a merge can never remove a key or an array element) |
| `--json` | machine-readable output |
| `--yes` | required for a cascading delete |

Flags may come before or after the positional arguments — both orders work.

**Exit codes:** `0` applied or no-op, `1` the operation failed, `2` bad
arguments or a refused operation. Applied and no-op are reported distinctly
(`✓` vs `∅`), because the underlying client returns success for both and
"nothing changed" is worth knowing.

Cascading deletes (`cmdb type delete`, `cmdb typeslink delete`,
`cmdb type subtype rm`) refuse to run without `--yes` and state the blast
radius first. There is no interactive prompt: a script that hangs on a
question is worse than one that fails loudly.

---

### `gwalk` — non-interactive graph traversal

#### `gwalk to <id>`

Walk to a vertex and persist the position:

```
foliage-cli gwalk to hub/network/router-1
```

#### `gwalk inspect`

Show the current vertex body and its link IDs:

```
foliage-cli gwalk inspect
foliage-cli gwalk inspect --pretty_print   # -p  JSON pretty print
foliage-cli gwalk inspect --all            # -a  include link bodies
```

#### `gwalk routes`

Print a tree of all routes reachable from the current position:

```
foliage-cli gwalk routes
foliage-cli gwalk routes --forward_depth 3 --backward_depth 1
foliage-cli gwalk routes --verbose 2       # 0=links, 1=+types, 2=+tags
```

#### `gwalk query <JPGQL>`

Run a JPGQL query from the current vertex:

```
foliage-cli gwalk query "out('contains').out('depends_on')"
```

#### `gwalk export`

Export the (sub)graph rooted at the current vertex:

```
foliage-cli gwalk export                         # graphml, full depth
foliage-cli gwalk export --format dot --depth 2
foliage-cli gwalk export --format graphml_json2xml --raw
foliage-cli gwalk export --exclude-vertex __meta --exclude-edge __meta
```

Flags:

| Flag | Short | Default | Description |
|---|---|---|---|
| `--format` | `-f` | `graphml` | Output format: `graphml`, `dot`, `graphml_json2xml` |
| `--depth` | `-d` | `-1` (all) | BFS depth limit |
| `--raw` | `-r` | false | Raw data only (no formatting) |
| `--exclude-vertex` | `-X` | — | Exclude vertex body fields (repeatable) |
| `--exclude-edge` | `-x` | — | Exclude edge body fields (repeatable) |

#### `gwalk import [filename]`

Upload a graph from a file or stdin:

```
foliage-cli gwalk import graph.graphml
foliage-cli gwalk import --stdin < graph.graphml
```

---

## Build

```
go build -o foliage-cli .
```

### Cross-compile for Linux/amd64

```
docker run -it --rm \
  -v ./:/go/src/github.com/foliagecp/cli \
  -w /go/src/github.com/foliagecp/cli \
  -e CGO_ENABLED=1 \
  docker.elastic.co/beats-dev/golang-crossbuild:1.21.1-main \
  --build-cmd "go build -o foliage-cli *.go" \
  -p "linux/amd64"
```

See [elastic/golang-crossbuild](https://github.com/elastic/golang-crossbuild) for available platforms.
