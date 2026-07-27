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

Opens a full-screen three-panel TUI. Navigation starts from the last persisted position (or `root` on first run).

```
┌──────────────────────────────────────────────────────────────────┐
│  ◈ hub/network/router-1                            loading 5…    │
├──────────────────┬───────────────────────────┬────────────────────┤
│ ← INCOMING (2)  │ ◈ router-1  [mytype]       │ → OUTGOING (5)    │
│                  │                            │                    │
│  managed_by  1   │  name:    Router 1         │  contains     3   │
│    admin ←       │  status:  active           │    port-a →       │
│                  │  ip:      10.0.0.1         │  ► port-b →       │
│                  │                            │    port-c →       │
│                  │  ──────────────────────    │                    │
│                  │  managed_by ← │ → contains │  depends_on   2   │
│                  │               │ → depends  │    upstream →     │
│                  │                            │    gateway →      │
└──────────────────┴───────────────────────────┴────────────────────┘
  h/l:panel  jk:nav  Enter:go  Tab:collapse  b:back  v:raw  q:quit
```

Press `?` inside the TUI for the full keymap — the status bar only has room for
the most-used bindings.

**Navigate and view**

| Key | Action |
|---|---|
| `h` / `l` | Switch focus between incoming and outgoing panels |
| `j` / `↓`, `k` / `↑` | Move cursor |
| `Enter` | Navigate to selected vertex; on a group header — toggle collapse |
| `Tab` | Toggle collapse of the current link-type group |
| `b` / `Backspace` | Go back (history stack) |
| `v` | Toggle raw JSON body vs. key-value view |
| `c` | Copy current vertex ID to clipboard |
| `/` | JPGQL query from current vertex |
| `f` | Live filter links by name · `Esc` clears |
| `e` | Export graph to file (graphml / dot / json2xml), choose depth |
| `r` / `Ctrl+R` | Refresh current vertex · also clear the entire cache |
| `R` | Jump to `root` and reset all state, including the anchor |
| `g` / `G` | Scroll body up / down |
| `?` | Show the full keymap |
| `q` / `Ctrl+C` | Quit |

**Create**

| Key | Action |
|---|---|
| `n` | New… — a menu of what can be created where you are standing |
| `a` | Anchor the current vertex as a link source (press again to clear) |
| `L` | Create a link from the anchor to the current vertex |

Linking is "walk, then link": press `a` on the source, navigate to the target
however you like, then press `L`. The anchor stays visible in the breadcrumb
row and survives navigation — it is data, not a mode. Inside the form, `←`/`→`
swaps the direction, so one anchor covers both.

Which API the link goes through is derived from the endpoints and shown in the
form title: type→type creates a **types-link** (a schema declaration), object→object
an **objects-link** (whose link type comes from that schema and is therefore not
editable), and anything else a **raw link**.

**Modify and delete**

| Key | Action |
|---|---|
| `i` | Edit a body — the current vertex's |
| `t` | Edit the tags of the selected link |
| `y` | Yank the displayed body, to reuse elsewhere |
| `x` | Toggle the low-level API — shown as `[LL]` in the header |
| `d` | Delete the selected link, or the vertex if the cursor is not on one |
| `D` | Delete the current vertex |

> **Note:** `d` previously scrolled the body half a page (an undocumented
> binding inherited from the viewport widget, duplicating `G`). It now deletes.

Inside the body editor: `Ctrl+S` apply · `Ctrl+R` switch MERGE ⇄ REPLACE ·
`Ctrl+E` open `$EDITOR` · `Esc` cancel. JSON is validated as you type and
submission is blocked while it is invalid. Machine-owned paths (`triggers`,
`cache`, …) are held aside and listed under the editor, so nothing is lost.

Deleting a **type** requires typing its name, because it removes every object
of that type. Structural vertices (`root`, `types`, `objects`, `trash_can`, …)
are refused outright.

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
