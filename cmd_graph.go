package main

import (
	"github.com/urfave/cli/v2"
)

// The `graph` command group — the low-level API, mirroring dbClient.Graph.
//
// These write raw vertices and edges with no CMDB semantics: no type links, no
// triggers, no schema validation. That is the point — it is the escape hatch
// for inspecting and repairing graph state that the high-level API would refuse
// to express. The `cmdb` group is the typed counterpart.

func graphCommand() *cli.Command {
	return &cli.Command{
		Name:  "graph",
		Usage: "low-level graph CRUD (raw vertices and links)",
		Subcommands: []*cli.Command{
			graphVertexCommand(),
			graphLinkCommand(),
		},
	}
}

// ── graph vertex ──────────────────────────────────────────────────────────────

func graphVertexCommand() *cli.Command {
	return &cli.Command{
		Name:  "vertex",
		Usage: "raw vertices",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "create a vertex",
				ArgsUsage: "<id>",
				Flags:     []cli.Flag{bodyFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "vertex")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					return reportOp(cCtx, "vertex.create", args[0], ops.vertexCreate(args[0], body))
				},
			},
			{
				Name:      "update",
				Usage:     "update a vertex body (defaults to the gwalk cursor)",
				ArgsUsage: "[id]",
				Flags: []cli.Flag{bodyFlag(), replaceFlag(), jsonFlag(),
					&cli.BoolFlag{Name: "upsert", Usage: "create the vertex if it does not exist"}},
				Action: func(cCtx *cli.Context) error {
					id, err := resolveID(cCtx, "vertex")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.vertexUpdate(id, body, cCtx.Bool("replace"), cCtx.Bool("upsert"))
					return reportOp(cCtx, "vertex.update", id, r)
				},
			},
			{
				Name:      "delete",
				Usage:     "delete a vertex and all of its links (defaults to the gwalk cursor)",
				ArgsUsage: "[id]",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					id, err := resolveID(cCtx, "vertex")
					if err != nil {
						return err
					}
					if err := refuseBuiltIn(id); err != nil {
						return err
					}
					return reportOp(cCtx, "vertex.delete", id, ops.vertexDelete(id))
				},
			},
			{
				Name:      "read",
				Usage:     "read a vertex with its links (defaults to the gwalk cursor)",
				ArgsUsage: "[id]",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					id, err := resolveID(cCtx, "vertex")
					if err != nil {
						return err
					}
					data, readErr := ops.vertexRead(id)
					return reportRead(cCtx, data, readErr)
				},
			},
		},
	}
}

// ── graph link ────────────────────────────────────────────────────────────────

// A link is addressable two ways — by its name (unique among the owner's
// out-links) or by its (to, type) pair (unique per direction). Update, delete
// and read accept either; create needs all four parts.
func graphLinkCommand() *cli.Command {
	nameFlag := &cli.StringFlag{Name: "name", Usage: "link name (unique among the source vertex's out-links)"}
	toFlag := &cli.StringFlag{Name: "to", Usage: "target vertex id"}
	typeFlag := &cli.StringFlag{Name: "type", Usage: "link type"}

	// pickSelector resolves which of the two addressing modes the user chose.
	pickSelector := func(cCtx *cli.Context) (byName bool, name, to, tp string, err error) {
		name, to, tp = cCtx.String("name"), cCtx.String("to"), cCtx.String("type")
		switch {
		case name != "":
			if err = validateID("link name", name); err != nil {
				return
			}
			return true, name, to, tp, nil
		case to != "" && tp != "":
			if err = validateID("target vertex", to); err != nil {
				return
			}
			return false, name, to, tp, nil
		default:
			err = usageErr("address the link either with --name, or with both --to and --type")
			return
		}
	}

	return &cli.Command{
		Name:  "link",
		Usage: "raw links between vertices",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "create a link (source defaults to the gwalk cursor)",
				ArgsUsage: "[from]",
				Flags: []cli.Flag{toFlag, nameFlag, typeFlag, tagsFlag(), bodyFlag(), jsonFlag(),
					&cli.BoolFlag{Name: "force", Usage: "overwrite an existing link, bypassing the uniqueness checks"}},
				Action: func(cCtx *cli.Context) error {
					from, err := resolveID(cCtx, "source vertex")
					if err != nil {
						return err
					}
					to, name, tp := cCtx.String("to"), cCtx.String("name"), cCtx.String("type")
					if to == "" || tp == "" {
						return usageErr("--to and --type are required to create a link")
					}
					if err := validateID("target vertex", to); err != nil {
						return err
					}
					if name == "" {
						// The server's own default: name the link after its target.
						name = stripDomain(to)
					}
					if err := validateID("link name", name); err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.linkCreate(from, to, name, tp, parseTags(cCtx), body, cCtx.Bool("force"))
					return reportOp(cCtx, "link.create", from+" -> "+to, r)
				},
			},
			{
				Name:      "update",
				Usage:     "update a link's body or tags (source defaults to the gwalk cursor)",
				ArgsUsage: "[from]",
				Flags:     []cli.Flag{nameFlag, toFlag, typeFlag, tagsFlag(), bodyFlag(), replaceFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					from, err := resolveID(cCtx, "source vertex")
					if err != nil {
						return err
					}
					byName, name, to, tp, err := pickSelector(cCtx)
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					tags := parseTags(cCtx)
					replace := cCtx.Bool("replace")
					if byName {
						r := ops.linkUpdate(from, name, tags, body, replace)
						return reportOp(cCtx, "link.update", from+":"+name, r)
					}
					r := ops.linkUpdateByToType(from, to, tp, tags, body, replace)
					return reportOp(cCtx, "link.update", from+" -"+tp+"-> "+to, r)
				},
			},
			{
				Name:      "delete",
				Usage:     "delete a link (source defaults to the gwalk cursor)",
				ArgsUsage: "[from]",
				Flags:     []cli.Flag{nameFlag, toFlag, typeFlag, jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					from, err := resolveID(cCtx, "source vertex")
					if err != nil {
						return err
					}
					byName, name, to, tp, err := pickSelector(cCtx)
					if err != nil {
						return err
					}
					if byName {
						return reportOp(cCtx, "link.delete", from+":"+name, ops.linkDelete(from, name))
					}
					return reportOp(cCtx, "link.delete", from+" -"+tp+"-> "+to, ops.linkDeleteByToType(from, to, tp))
				},
			},
			{
				Name:      "read",
				Usage:     "read a link (source defaults to the gwalk cursor)",
				ArgsUsage: "[from]",
				Flags:     []cli.Flag{nameFlag, toFlag, typeFlag, jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					from, err := resolveID(cCtx, "source vertex")
					if err != nil {
						return err
					}
					byName, name, to, tp, err := pickSelector(cCtx)
					if err != nil {
						return err
					}
					if byName {
						data, readErr := ops.linkRead(from, name)
						return reportRead(cCtx, data, readErr)
					}
					data, readErr := ops.linkReadByToType(from, to, tp)
					return reportRead(cCtx, data, readErr)
				},
			},
		},
	}
}
