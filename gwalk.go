package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/xlab/treeprint"
)

const (
	gWalkFileName = "gwalk"
)

var (
	gWalkData *easyjson.JSON
)

func gWalkLoad() error {
	data, err := os.ReadFile(fmt.Sprintf("%s/%s", FoliageCLIDir, gWalkFileName))
	if err != nil {
		if os.IsNotExist(err) {
			gWalkData = easyjson.NewJSONObject().GetPtr()
			return nil
		}
		return err
	}
	j, ok := easyjson.JSONFromBytes(data)
	if !ok {
		return fmt.Errorf("gWalkLoad: invalid gwalk data, must be a json")
	}
	gWalkData = &j
	return nil
}

func gWalkSave() error {
	if err := os.MkdirAll(FoliageCLIDir, os.ModePerm); err != nil {
		return err
	}
	b := gWalkData.ToBytes()
	err := os.WriteFile(fmt.Sprintf("%s/%s", FoliageCLIDir, gWalkFileName), b, 0644)
	if err != nil {
		return err
	}

	return nil
}

func gWalkTo(id string) error {
	if loadErr := gWalkLoad(); loadErr != nil {
		return loadErr
	}
	if !gWalkData.SetByPath("id", easyjson.NewJSON(id)) {
		return fmt.Errorf("cannot walk to the vertex with id=%s, json update failed", id)
	}
	if saveErr := gWalkSave(); saveErr != nil {
		return saveErr
	}
	return nil
}

func gWalkInspect(prettyPrint bool) error {
	const prefixIndent = "  "
	gWalkLoad()
	j, err := natsRequest("functions.cli.graph.vertex.info", gWalkData.GetByPath("id").AsStringDefault("root"), nil, nil)
	if err != nil {
		return err
	}

	id := j.GetByPath("id").AsStringDefault("???")
	fmt.Println("Vertex")
	fmt.Println(prefixIndent, id)
	fmt.Println()

	body := j.GetByPath("body")
	fmt.Println("Body")
	if body.IsNonEmptyObject() {
		if prettyPrint {
			fmt.Println(prefixIndent, JSONStrPrettyStringAnyway(body.ToString(), prefixIndent, "  "))
		} else {
			fmt.Println(body.ToString(), prefixIndent, "  ")
		}
	} else {
		fmt.Println(prefixIndent, "-")
	}
	fmt.Println()

	outputLinks := j.GetByPath("links.output")
	if outputLinks.IsArray() && outputLinks.ArraySize() > 0 {
		fmt.Println("Output Links")

		for i := 0; i < outputLinks.ArraySize(); i++ {
			outputLink := outputLinks.ArrayElement(i)
			if outputLink.IsNonEmptyObject() {
				id := outputLink.GetByPath("id").AsStringDefault("???")
				fmt.Println(prefixIndent, "To: ", id)

				t := outputLink.GetByPath("type").AsStringDefault("???")
				fmt.Println(prefixIndent, "Type: ", t)

				if tags, ok := outputLink.GetByPath("tags").AsArrayString(); ok {
					fmt.Println(prefixIndent, "Tags: ", strings.Join(tags, " "))
				} else {
					fmt.Println(prefixIndent, "Tags: -")
				}

				linkBody := outputLink.GetByPath("body")
				if body.IsNonEmptyObject() {
					if prettyPrint {
						fmt.Printf("%s Body: %s\n", prefixIndent, JSONStrPrettyStringAnyway(linkBody.ToString(), prefixIndent+"  ", "  "))
					} else {
						fmt.Println(prefixIndent, "Body: ", linkBody.ToString())
					}
				} else {
					fmt.Println(prefixIndent, "Body: -")
				}
				fmt.Println()
			}
		}
	}

	inputLinks := j.GetByPath("links.input")
	if inputLinks.IsArray() && inputLinks.ArraySize() > 0 {
		fmt.Println("Input Links")

		for i := 0; i < inputLinks.ArraySize(); i++ {
			inputLink := inputLinks.ArrayElement(i)
			if inputLink.IsNonEmptyObject() {
				id := inputLink.GetByPath("id").AsStringDefault("???")
				fmt.Println(prefixIndent, "From: ", id)

				t := inputLink.GetByPath("type").AsStringDefault("???")
				fmt.Println(prefixIndent, "Type: ", t)

				if tags, ok := inputLink.GetByPath("tags").AsArrayString(); ok {
					fmt.Println(prefixIndent, "Tags: ", strings.Join(tags, " "))
				} else {
					fmt.Println(prefixIndent, "Tags: -")
				}

				linkBody := inputLink.GetByPath("body")
				if body.IsNonEmptyObject() {
					if prettyPrint {
						fmt.Printf("%s Body: %s\n", prefixIndent, JSONStrPrettyStringAnyway(linkBody.ToString(), prefixIndent+"  ", "  "))
					} else {
						fmt.Println(prefixIndent, "Body: ", linkBody.ToString())
					}
				} else {
					fmt.Println(prefixIndent, "Body: -")
				}
				fmt.Println()
			}
		}
	}

	return nil
}

func gWalkRoutes(fd, bd int, verbose int) error {
	gWalkLoad()
	payload := easyjson.NewJSONObject()
	payload.SetByPath("fd", easyjson.NewJSON(fd))
	payload.SetByPath("bd", easyjson.NewJSON(bd))
	id := gWalkData.GetByPath("id").AsStringDefault("root")
	routesJson, err := natsRequest("functions.cli.graph.vertex.routes", id, &payload, nil)
	if err != nil {
		return err
	}

	// Print routes as a tree -----------------------------------------------------------
	tree := treeprint.New()
	tree.SetValue(id)

	processTree := func(tree treeprint.Tree, transitionStr string) {
		stack := []*easyjson.JSON{routesJson}
		treeStack := []treeprint.Tree{tree}
		for len(stack) > 0 {
			rj := stack[0]
			stack = stack[1:]

			tree := treeStack[0]
			treeStack = treeStack[1:]

			outsJson := rj.GetByPath(transitionStr)
			if outsJson.IsArray() {
				if verbose >= 1 {
					for i := 0; i < outsJson.ArraySize(); i++ {
						outJson := outsJson.ArrayElement(i)
						outToId := outJson.GetByPath("id").AsStringDefault("unknown")
						linkType := outJson.GetByPath("type").AsStringDefault("unknown")

						linkMeta := ""
						if verbose >= 2 {
							if tags, ok := outJson.GetByPath("tags").AsArrayString(); ok && outJson.GetByPath("tags").ArraySize() > 0 {
								linkMeta = fmt.Sprintf("%s | #%v", linkType, strings.Join(tags, " #"))
							} else {
								linkMeta = fmt.Sprintf("%s", linkType)
							}
						} else {
							linkMeta = fmt.Sprintf("%s", linkType)
						}
						newTree := tree.AddMetaBranch(linkMeta, outToId)

						stack = append(stack, &outJson)
						treeStack = append(treeStack, newTree)
					}
				} else {
					uniqueIds := map[string]bool{}
					for i := 0; i < outsJson.ArraySize(); i++ {
						outJson := outsJson.ArrayElement(i)
						outToId := outJson.GetByPath("id").AsStringDefault("unknown")

						if _, ok := uniqueIds[outToId]; !ok {
							newTree := tree.AddBranch(outToId)

							stack = append(stack, &outJson)
							treeStack = append(treeStack, newTree)

							uniqueIds[outToId] = true
						}
					}
				}
			}
		}
	}

	if fd > 0 {
		outs := tree.AddMetaBranch(fmt.Sprintf("depth=%d", fd), "OUT")
		processTree(outs, "outs")
	}

	if bd > 0 {
		ins := tree.AddMetaBranch(fmt.Sprintf("depth=%d", bd), "IN")
		processTree(ins, "ins")
	}

	fmt.Println(tree.String())
	// ----------------------------------------------------------------------------------

	//fmt.Println(JSONStrPrettyStringAnyway(routesJson.ToString()))
	return nil
}

func gWalkQuery(algorithm string, query string) error {
	const prefixIndent = "  "

	gWalkLoad()
	payload := easyjson.NewJSONObjectWithKeyValue("jpgql_query", easyjson.NewJSON(query))
	j, err := natsRequest(fmt.Sprintf("functions.graph.api.query.jpgql.%s", algorithm), gWalkData.GetByPath("id").AsStringDefault("root"), &payload, nil)
	if err != nil {
		return err
	}
	fmt.Println("Result:", strings.Join(j.GetByPath("result").ObjectKeys(), ", "))

	return nil
}
