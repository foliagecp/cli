# Foliage CLI

Command-line tool for navigating and managing the [Foliage](https://github.com/foliagecp) graph database over NATS.

### Build from source

```bash
go build -o foliage-cli .
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `NATS_URL` | `nats://nats:foliage@nats:4222` | NATS server URL |
| `NATS_HUB_DOMAIN` | `hub` | Hub domain |
| `NATS_REQUEST_TIMEOUT_SEC` | `60` | Request timeout in seconds |
| `FOLIAGE_CLI_DIR` | `~/.foliage-cli` | State directory |

## State

Current vertex is persisted across sessions in `~/.foliage-cli/gwalk`. Defaults to `root` if not set.

---

## TUI

```bash
foliage-cli tui
```

Opens a full-screen terminal interface for navigating the graph. Defaults to vertex `root` on first launch.

```
◈ root  ← parent_vertex
┌─ ◈ root ────────────────┐┌─ Links  ↓out:3  ↑in:1 ──────────┐
│ {                       ││  → contains   child1              │
│   "type": "folder",     ││▶ → ref        config  [ref]       │
│   "name": "Root"        ││  ← parent     root                │
│ }                       ││                                   │
└─────────────────────────┘└───────────────────────────────────┘
jk:navigate  Enter:go  b:back  a:all  /:query  r:refresh  g/G:body ↑↓ half  q:quit
```

The left panel shows the current vertex body (scroll half-page with `g`/`G` or use the mouse wheel). The right panel lists outgoing (`→`) and incoming (`←`) links. The current vertex ID is always visible in the header.

On narrow terminals (< 60 cols) the TUI switches to a compact single-column layout automatically.

### Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Next link (wraps to top) |
| `k` / `↑` | Previous link (wraps to bottom) |
| `Enter` | Navigate to selected vertex |
| `b` / `Backspace` | Go back (history) |
| `a` | Toggle all-details mode: show link types, tags, and bodies in the body panel |
| `g` / `G` | Scroll body panel up / down half page |
| `/` | JPGQL query — results appear as a navigable list in the right panel |
| `r` | Refresh / retry connection |
| `q` | Quit |

### Query results mode

After a `/` query executes, the right panel switches to result navigation:

| Key | Action |
|-----|--------|
| `j` / `↓` | Next result |
| `k` / `↑` | Previous result |
| `Enter` | Navigate to selected result vertex |
| `Esc` / `b` | Close results, return to link list |

---

## gwalk

One-off commands for scripting and non-interactive use.

```bash
foliage-cli gwalk to <id>
foliage-cli gwalk inspect [--pretty_print] [--all]
foliage-cli gwalk routes [--forward_depth N] [--backward_depth N] [--verbose N]
foliage-cli gwalk query "<JPGQL expression>"
foliage-cli gwalk export [--format graphml|dot] [--depth N] [--raw]
foliage-cli gwalk export --exclude-vertex __meta --exclude-edge spec.secret
foliage-cli gwalk import [--format graphml] <filename>
foliage-cli gwalk import --stdin < graph.xml
```

---

## Cross-compilation

```bash
docker run -it --rm \
  -v ./:/go/src/github.com/foliagecp/cli \
  -w /go/src/github.com/foliagecp/cli \
  -e CGO_ENABLED=1 \
  docker.elastic.co/beats-dev/golang-crossbuild:1.21.1-main \
  --build-cmd "go build -o foliage-cli *.go" \
  -p "linux/amd64"
```

See [golang-crossbuild](https://github.com/elastic/golang-crossbuild) for other target platforms.