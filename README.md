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

**Keybindings:**

| Key | Action |
|---|---|
| `h` / `l` | Switch focus between incoming and outgoing panels |
| `j` / `↓` | Move cursor down |
| `k` / `↑` | Move cursor up |
| `Enter` | Navigate to selected vertex; on a group header — toggle collapse |
| `Tab` | Toggle collapse of the current link-type group |
| `b` / `Backspace` | Go back (history stack) |
| `v` | Toggle raw JSON body vs. key-value view |
| `c` | Copy current vertex ID to clipboard |
| `/` | JPGQL query from current vertex |
| `f` | Live filter links by name |
| `Esc` | Clear active filter |
| `e` | Export graph to file (graphml / dot / json2xml), choose depth |
| `r` | Refresh current vertex (evicts from cache) |
| `Ctrl+R` | Refresh and clear the entire cache |
| `R` | Jump to `root` and reset all state |
| `g` / `G` | Scroll body up / down |
| `q` / `Ctrl+C` | Quit |

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
