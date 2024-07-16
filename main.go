package main

import (
	"fmt"
	"log"
	"os"

	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun/system"
	"github.com/urfave/cli/v2"
)

var (
	NatsURL               string = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
	NatsHubDomain         string = system.GetEnvMustProceed("NATS_HUB_DOMAIN", "hub")
	NatsRequestTimeoutSec int    = system.GetEnvMustProceed("NATS_REQUEST_TIMEOUT_SEC", 60)
	FoliageCLIDir         string = system.GetEnvMustProceed("FOLIAGE_CLI_DIR", "~/.foliage-cli")

	dbClient db.DBSyncClient
)

func main() {
	dbc, err := db.NewDBSyncClient(NatsURL, NatsRequestTimeoutSec, NatsHubDomain)
	if err != nil {
		log.Fatalln(err)
	}
	dbClient = dbc

	if s, err := expandFileName(FoliageCLIDir); err != nil {
		log.Panicln(err)
	} else {
		FoliageCLIDir = s
	}

	app := &cli.App{
		Name:  "foliage-cli",
		Usage: "Foliage command line interface",
		Commands: []*cli.Command{
			{
				Name:  "gwalk",
				Usage: "traverse the graph",
				Subcommands: []*cli.Command{
					{
						Name:      "to",
						Usage:     "walk to specified vertex id",
						ArgsUsage: "<id>",
						Action: func(cCtx *cli.Context) error {
							if cCtx.NArg() != 1 {
								return fmt.Errorf("wrong argument amount")
							}
							if err := gWalkTo(cCtx.Args().First()); err != nil {
								return err
							}
							return nil
						},
					},
					{
						Name:  "routes",
						Usage: "find all existing routes",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "forward_depth",
								Aliases: []string{"fd"},
								Value:   1,
								Usage:   "Forward depth",
							},
							&cli.IntFlag{
								Name:    "backward_depth",
								Aliases: []string{"bd"},
								Value:   0,
								Usage:   "Backward depth",
							},
							&cli.IntFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Value:   0,
								Usage:   "Level of details (0 - just links, 1 - links types, 2 - link types and tags)",
							},
						},
						Action: func(cCtx *cli.Context) error {
							return gWalkRoutes(cCtx.Uint("fd"), cCtx.Uint("bd"), cCtx.Int("v"))
						},
					},
					{
						Name:  "inspect",
						Usage: "show detailed info about current location",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "pretty_print",
								Aliases: []string{"p"},
								Value:   false,
								Usage:   "JSON pretty print",
							},
							&cli.BoolFlag{
								Name:    "all",
								Aliases: []string{"a"},
								Value:   false,
								Usage:   "Print all available data",
							},
						},
						Action: func(cCtx *cli.Context) error {
							return gWalkInspect(cCtx.Bool("p"), cCtx.Bool("a"))
						},
					},
					{
						Name:      "query",
						Usage:     "JPGQL query from current gwalk id",
						ArgsUsage: "<JPGQL query>",
						Action: func(cCtx *cli.Context) error {
							if cCtx.NArg() != 1 {
								return fmt.Errorf("wrong argument amount")
							}
							return gWalkQuery(cCtx.Args().First())
						},
					},
					{
						Name:  "export",
						Usage: "Receive graph in different formats",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "format",
								Aliases: []string{"f"},
								Value:   "dot",
								Usage:   "Use one from the following: dot, graphml, graphml_json2xml. Default: dot",
							},
							&cli.IntFlag{
								Name:    "depth",
								Aliases: []string{"d"},
								Value:   -1,
								Usage:   "Graph depth",
							},
							&cli.BoolFlag{
								Name:    "raw",
								Aliases: []string{"r"},
								Value:   false,
								Usage:   "Raw DOT data only",
							},
						},
						Action: func(cCtx *cli.Context) error {
							return gWalkPrintGraph(cCtx.String("f"), cCtx.Int("d"), cCtx.Bool("r"))
						},
					},
				},
			},
		},
	}

	if err = app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// --------------------------------------------------------------------------------------
