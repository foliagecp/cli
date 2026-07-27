package main

import (
	"github.com/urfave/cli/v2"
)

// The `cmdb` command group — the high-level API, mirroring dbClient.CMDB.
//
// The model, in short:
//   - a TYPE is a vertex in the `types` topology; it defines a category
//   - an OBJECT is an instance of a type
//   - a TYPES-LINK between two types is a schema declaration: its body carries
//     the link type that object-links between instances of those two types will
//     receive. Without it, creating an object-link between two objects FAILS —
//     the link type is derived from the schema and is never chosen by the caller
//   - a SUBTYPE relation lets an object of a child type be linked as though it
//     were an instance of the parent, via the `objectslink super` commands
//
// The `graph` group is the untyped counterpart.

func cmdbCommand() *cli.Command {
	return &cli.Command{
		Name:  "cmdb",
		Usage: "high-level CRUD (types, objects and their links)",
		Subcommands: []*cli.Command{
			cmdbTypeCommand(),
			cmdbTypesLinkCommand(),
			cmdbObjectCommand(),
			cmdbObjectsLinkCommand(),
		},
	}
}

// ── cmdb type ─────────────────────────────────────────────────────────────────

func cmdbTypeCommand() *cli.Command {
	return &cli.Command{
		Name:  "type",
		Usage: "object types",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "create a type",
				ArgsUsage: "<name>",
				Flags:     []cli.Flag{bodyFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "type")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					return reportOp(cCtx, "type.create", args[0], ops.typeCreate(args[0], body))
				},
			},
			{
				Name:      "update",
				Usage:     "update a type body",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{bodyFlag(), replaceFlag(), jsonFlag(),
					&cli.BoolFlag{Name: "upsert", Usage: "create the type if it does not exist"}},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "type")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.typeUpdate(args[0], body, cCtx.Bool("replace"), cCtx.Bool("upsert"))
					return reportOp(cCtx, "type.update", args[0], r)
				},
			},
			{
				Name:      "delete",
				Usage:     "delete a type AND every object of that type",
				ArgsUsage: "<name>",
				Flags:     []cli.Flag{yesFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "type")
					if err != nil {
						return err
					}
					if err := refuseBuiltIn(args[0]); err != nil {
						return err
					}
					// Cascading: DeleteType walks every instance and deletes it.
					if err := confirmCascade(cCtx, "deleting type", args[0], objectCountPhrase(args[0])); err != nil {
						return err
					}
					return reportOp(cCtx, "type.delete", args[0], ops.typeDelete(args[0]))
				},
			},
			{
				Name:      "read",
				Usage:     "read a type, its sub-types and its object ids",
				ArgsUsage: "<name>",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "type")
					if err != nil {
						return err
					}
					data, readErr := ops.typeRead(args[0])
					return reportRead(cCtx, data, readErr)
				},
			},
			{
				Name:  "subtype",
				Usage: "type inheritance",
				Subcommands: []*cli.Command{
					{
						Name:      "add",
						Usage:     "declare <child> a sub-type of <base>",
						ArgsUsage: "<base> <child>",
						Flags:     []cli.Flag{jsonFlag()},
						Action: func(cCtx *cli.Context) error {
							args, err := requireArgs(cCtx, "base type", "child type")
							if err != nil {
								return err
							}
							r := ops.subTypeSet(args[0], args[1])
							return reportOp(cCtx, "type.subtype.add", args[0]+" -> "+args[1], r)
						},
					},
					{
						Name:      "rm",
						Usage:     "remove the sub-type relation between <base> and <child>",
						ArgsUsage: "<base> <child>",
						Flags:     []cli.Flag{yesFlag(), jsonFlag()},
						Action: func(cCtx *cli.Context) error {
							args, err := requireArgs(cCtx, "base type", "child type")
							if err != nil {
								return err
							}
							// Cascading: dropping the relation invalidates
							// inheritance for every descendant and every
							// claimed-type link that relied on it.
							if err := confirmCascade(cCtx, "removing sub-type", args[0]+" -> "+args[1],
								"invalidates inherited links on every descendant type"); err != nil {
								return err
							}
							r := ops.subTypeRemove(args[0], args[1])
							return reportOp(cCtx, "type.subtype.rm", args[0]+" -> "+args[1], r)
						},
					},
				},
			},
		},
	}
}

// ── cmdb typeslink ────────────────────────────────────────────────────────────

func cmdbTypesLinkCommand() *cli.Command {
	return &cli.Command{
		Name:  "typeslink",
		Usage: "schema links between types",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "declare that objects of <from> may link to objects of <to>",
				ArgsUsage: "<fromType> <toType>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "object-link-type",
						Usage:    "the link type that object-links between instances will get",
						Required: true,
					},
					tagsFlag(), bodyFlag(), jsonFlag(),
				},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from type", "to type")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					olt := cCtx.String("object-link-type")
					r := ops.typesLinkCreate(args[0], args[1], olt, parseTags(cCtx), body)
					return reportOp(cCtx, "typeslink.create", args[0]+" -"+olt+"-> "+args[1], r)
				},
			},
			{
				Name:      "update",
				Usage:     "update a types-link body or tags",
				ArgsUsage: "<fromType> <toType>",
				Flags:     []cli.Flag{tagsFlag(), bodyFlag(), replaceFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from type", "to type")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.typesLinkUpdate(args[0], args[1], parseTags(cCtx), body, cCtx.Bool("replace"))
					return reportOp(cCtx, "typeslink.update", args[0]+" -> "+args[1], r)
				},
			},
			{
				Name:      "delete",
				Usage:     "delete a types-link AND the corresponding link on every instance",
				ArgsUsage: "<fromType> <toType>",
				Flags:     []cli.Flag{yesFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from type", "to type")
					if err != nil {
						return err
					}
					// Cascading: DeleteTypesLink walks every instance of the
					// source type and removes the matching object-link.
					if err := confirmCascade(cCtx, "deleting types-link", args[0]+" -> "+args[1],
						objectCountPhrase(args[0])+"' links of this type"); err != nil {
						return err
					}
					r := ops.typesLinkDelete(args[0], args[1])
					return reportOp(cCtx, "typeslink.delete", args[0]+" -> "+args[1], r)
				},
			},
			{
				Name:      "read",
				Usage:     "read a types-link (its body.type is the object-link type)",
				ArgsUsage: "<fromType> <toType>",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from type", "to type")
					if err != nil {
						return err
					}
					data, readErr := ops.typesLinkRead(args[0], args[1])
					return reportRead(cCtx, data, readErr)
				},
			},
		},
	}
}

// ── cmdb object ───────────────────────────────────────────────────────────────

func cmdbObjectCommand() *cli.Command {
	return &cli.Command{
		Name:  "object",
		Usage: "typed objects",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "create an object of a type",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "type", Usage: "the object's type", Required: true},
					bodyFlag(), jsonFlag(),
				},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "object")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.objectCreate(args[0], cCtx.String("type"), body)
					return reportOp(cCtx, "object.create", args[0], r)
				},
			},
			{
				Name:      "update",
				Usage:     "update an object body (defaults to the gwalk cursor)",
				ArgsUsage: "[id]",
				Flags: []cli.Flag{bodyFlag(), replaceFlag(), jsonFlag(),
					&cli.StringFlag{Name: "upsert-type", Usage: "create the object with this type if it does not exist"}},
				Action: func(cCtx *cli.Context) error {
					id, err := resolveID(cCtx, "object")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.objectUpdate(id, body, cCtx.Bool("replace"), cCtx.String("upsert-type"))
					return reportOp(cCtx, "object.update", id, r)
				},
			},
			{
				Name:      "delete",
				Usage:     "delete an object (defaults to the gwalk cursor)",
				ArgsUsage: "[id]",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					id, err := resolveID(cCtx, "object")
					if err != nil {
						return err
					}
					if err := refuseBuiltIn(id); err != nil {
						return err
					}
					return reportOp(cCtx, "object.delete", id, ops.objectDelete(id))
				},
			},
			{
				Name:      "read",
				Usage:     "read an object with its type and neighbours (defaults to the gwalk cursor)",
				ArgsUsage: "[id]",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					id, err := resolveID(cCtx, "object")
					if err != nil {
						return err
					}
					data, readErr := ops.objectRead(id)
					return reportRead(cCtx, data, readErr)
				},
			},
		},
	}
}

// ── cmdb objectslink ──────────────────────────────────────────────────────────

func cmdbObjectsLinkCommand() *cli.Command {
	return &cli.Command{
		Name:  "objectslink",
		Usage: "links between objects (the link type comes from the schema)",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "link two objects; requires a types-link between their types",
				ArgsUsage: "<from> <to>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "link name (defaults to the target id)"},
					tagsFlag(), bodyFlag(), jsonFlag(),
				},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from object", "to object")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.objectsLinkCreate(args[0], args[1], cCtx.String("name"), parseTags(cCtx), body)
					return reportOp(cCtx, "objectslink.create", args[0]+" -> "+args[1], r)
				},
			},
			{
				Name:      "update",
				Usage:     "update an object-link's body or tags",
				ArgsUsage: "<from> <to>",
				Flags:     []cli.Flag{tagsFlag(), bodyFlag(), replaceFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from object", "to object")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.objectsLinkUpdate(args[0], args[1], parseTags(cCtx), body, cCtx.Bool("replace"))
					return reportOp(cCtx, "objectslink.update", args[0]+" -> "+args[1], r)
				},
			},
			{
				Name:      "delete",
				Usage:     "delete an object-link",
				ArgsUsage: "<from> <to>",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from object", "to object")
					if err != nil {
						return err
					}
					r := ops.objectsLinkDelete(args[0], args[1])
					return reportOp(cCtx, "objectslink.delete", args[0]+" -> "+args[1], r)
				},
			},
			{
				Name:      "read",
				Usage:     "read an object-link with its endpoint types",
				ArgsUsage: "<from> <to>",
				Flags:     []cli.Flag{jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from object", "to object")
					if err != nil {
						return err
					}
					data, readErr := ops.objectsLinkRead(args[0], args[1])
					return reportRead(cCtx, data, readErr)
				},
			},
			cmdbObjectsLinkSuperCommand(),
		},
	}
}

// A supertype link lets two objects be linked "as if" they were instances of
// types they inherit from. The claimed types are validated server-side and the
// resulting link type becomes the compound "<fromClaim>#<toClaim>#<rel>", so
// several such links can coexist between the same object pair.
func cmdbObjectsLinkSuperCommand() *cli.Command {
	fromClaim := &cli.StringFlag{Name: "from-claim", Usage: "type claimed for the source object", Required: true}
	toClaim := &cli.StringFlag{Name: "to-claim", Usage: "type claimed for the target object", Required: true}

	return &cli.Command{
		Name:  "super",
		Usage: "object links through claimed super-types",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "link two objects under claimed types",
				ArgsUsage: "<from> <to>",
				Flags: []cli.Flag{fromClaim, toClaim,
					&cli.StringFlag{Name: "name", Usage: "link name (blank lets the server default it)"},
					tagsFlag(), bodyFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from object", "to object")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.superLinkCreate(args[0], args[1],
						cCtx.String("from-claim"), cCtx.String("to-claim"),
						cCtx.String("name"), parseTags(cCtx), body)
					return reportOp(cCtx, "objectslink.super.create", args[0]+" -> "+args[1], r)
				},
			},
			{
				Name:      "update",
				Usage:     "update a claimed-type object link",
				ArgsUsage: "<from> <to>",
				Flags: []cli.Flag{fromClaim, toClaim,
					&cli.StringFlag{Name: "name", Usage: "link name"},
					tagsFlag(), bodyFlag(), replaceFlag(), jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from object", "to object")
					if err != nil {
						return err
					}
					body, err := parseBody(cCtx)
					if err != nil {
						return err
					}
					r := ops.superLinkUpdate(args[0], args[1],
						cCtx.String("from-claim"), cCtx.String("to-claim"),
						cCtx.String("name"), parseTags(cCtx), body, cCtx.Bool("replace"))
					return reportOp(cCtx, "objectslink.super.update", args[0]+" -> "+args[1], r)
				},
			},
			{
				Name:      "delete",
				Usage:     "delete a claimed-type object link",
				ArgsUsage: "<from> <to>",
				Flags:     []cli.Flag{fromClaim, toClaim, jsonFlag()},
				Action: func(cCtx *cli.Context) error {
					args, err := requireArgs(cCtx, "from object", "to object")
					if err != nil {
						return err
					}
					r := ops.superLinkDelete(args[0], args[1],
						cCtx.String("from-claim"), cCtx.String("to-claim"))
					return reportOp(cCtx, "objectslink.super.delete", args[0]+" -> "+args[1], r)
				},
			},
		},
	}
}
