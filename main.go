package main

import (
	"fmt"
	"log"
	"os"

	//"github.com/foliagecp/sdk/statefun"
	//"github.com/foliagecp/sdk/statefun/cache"

	"github.com/foliagecp/sdk/statefun/system"
	"github.com/nats-io/nats.go"
	"github.com/urfave/cli/v2"
)

var (
	NatsURL               string = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
	NatsRequestTimeoutSec int    = system.GetEnvMustProceed("NATS_REQUEST_TIMEOUT_SEC", 60)
	FoliageCLIDir         string = system.GetEnvMustProceed("FOLIAGE_CLI_DIR", "~/.foliage-cli")

	nc *nats.Conn
)

func main() {
	var err error
	nc, err = nats.Connect(NatsURL)
	if err != nil {
		log.Fatal(err)
	}

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
						Action: func(cCtx *cli.Context) error {
							fmt.Println("removed task template: ", cCtx.Args().First())
							return nil
						},
					},
					{
						Name:  "inspect",
						Usage: "show detailed info about current location",
						Action: func(cCtx *cli.Context) error {
							fmt.Println("removed task template: ", cCtx.Args().First())
							return nil
						},
					},
					{
						Name:      "query",
						Usage:     "JPGQL query from current gwalk id",
						ArgsUsage: "<JPGQL query>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "alg",
								Value: "dcra",
								Usage: "JPGQL algorithm: <dcra|ctra>. Call Tree Result Aggregation or Direct Cache Result Aggregation",
							},
						},
						Action: func(cCtx *cli.Context) error {
							s := cCtx.String("alg")
							algorithm := "dcra"
							if s != "ctra" && s != "dcra" {
								fmt.Printf("Invalig JPGQL algorithm specified %s, required <dcra|ctra>. Will use %s as the default one.\n", s, algorithm)
							} else {
								algorithm = s
							}
							if cCtx.NArg() != 1 {
								return fmt.Errorf("wrong argument amount")
							}
							return gWalkQuery(algorithm, cCtx.Args().First())
						},
					},
				},
			},
		},
	}

	if err = app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
	/*onAfterStart := func(runtime *statefun.Runtime) error {
		return nil
	}

	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(NatsURL, "cli")); err == nil {
		if err := runtime.Start(cache.NewCacheConfig(), onAfterStart); err != nil {
			fmt.Printf("Cannot start due to an error: %s\n", err)
		}
	} else {
		fmt.Printf("Cannot create statefun runtime due to an error: %s\n", err)
	}*/
}

// --------------------------------------------------------------------------------------
