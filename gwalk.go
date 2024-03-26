package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/xlab/treeprint"
)

type linkId struct {
	from string
	name string
}

func (l linkId) asStr() string {
	return l.from + ":" + l.name
}

type fullLinkInfo struct {
	id   linkId
	body *easyjson.JSON
	to   string
	tp   string
	tags []string
}

type fullVertexInfo struct {
	id       string
	body     *easyjson.JSON
	outLinks []linkId
	inLinks  []linkId
}

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

func getLinkFullInfo(lid linkId) (fli fullLinkInfo, resErr error) {
	fli.id = lid
	fli.tags = []string{}

	data, err := dbClient.Graph.VerticesLinkRead(fli.id.from, fli.id.name, true)
	if err != nil {
		resErr = err
		return
	}

	to := data.GetByPath("to").AsStringDefault("")
	if len(to) == 0 {
		resErr = fmt.Errorf("link's to vertex id is empty invalid")
		return
	}

	fli.to = to
	fli.body = data.GetByPath("body").GetPtr()
	fli.tp = data.GetByPath("type").AsStringDefault("")
	if arr, ok := data.GetByPath("tags").AsArrayString(); ok {
		fli.tags = arr
	}

	return
}

func getVertexFullInfo(vertexId string) (fvi fullVertexInfo, resErr error) {
	fvi.id = vertexId
	fvi.outLinks = []linkId{}
	fvi.inLinks = []linkId{}

	data, err := dbClient.Graph.VertexRead(vertexId, true)
	if err != nil {
		resErr = err
		return
	}

	fvi.body = data.GetByPath("body").GetPtr()

	if arr, ok := data.GetByPath("links.out.names").AsArrayString(); ok {
		for _, oln := range arr {
			fvi.outLinks = append(fvi.outLinks, linkId{vertexId, oln})
		}
	}

	inLinks := data.GetByPath("links.in").GetPtr()
	for i := 0; i < inLinks.ArraySize(); i++ {
		inLink := inLinks.ArrayElement(i)
		from := inLink.GetByPath("from").AsStringDefault("")
		linkName := inLink.GetByPath("name").AsStringDefault("")
		fvi.inLinks = append(fvi.inLinks, linkId{from, linkName})
	}

	return
}

func gWalkInspect(prettyPrint bool) error {
	const prefixIndent = "  "
	gWalkLoad()

	fvi, err := getVertexFullInfo(gWalkData.GetByPath("id").AsStringDefault("root"))
	if err != nil {
		return err
	}

	fmt.Println("Vertex")
	fmt.Println(prefixIndent, fvi.id)
	fmt.Println()

	fmt.Println("Body")
	if fvi.body.IsNonEmptyObject() {
		if prettyPrint {
			fmt.Println(prefixIndent + JSONStrPrettyStringAnyway(fvi.body, len(prefixIndent), 2))
		} else {
			fmt.Println(prefixIndent, fvi.body.ToString())
		}
	} else {
		fmt.Println(prefixIndent, "-")
	}
	fmt.Println()

	printLink := func(fli fullLinkInfo, input bool) {
		if input {
			fmt.Println(prefixIndent+"From: ", fli.id.from)
		}
		fmt.Println(prefixIndent+"Name: ", fli.id.name)
		if !input {
			fmt.Println(prefixIndent+"To: ", fli.to)
		}
		fmt.Println(prefixIndent+"Type: ", fli.tp)
		fmt.Println(prefixIndent+"Tags: ", strings.Join(fli.tags, " "))
		linkBody := fli.body
		if linkBody.IsNonEmptyObject() {
			if prettyPrint {
				fmt.Printf("%sBody: %s\n", prefixIndent, JSONStrPrettyStringAnyway(linkBody, len(prefixIndent)*2, 2))
			} else {
				fmt.Println(prefixIndent+"Body: ", linkBody.ToString())
			}
		} else {
			fmt.Println(prefixIndent + "Body:")
		}
		fmt.Println()
	}

	if len(fvi.outLinks) > 0 {
		fmt.Println("Output Links")
	}
	for _, lid := range fvi.outLinks {
		fli, err := getLinkFullInfo(lid)
		if err == nil {
			printLink(fli, false)
		}
	}
	if len(fvi.inLinks) > 0 {
		fmt.Println("Input Links")
	}
	for _, lid := range fvi.inLinks {
		fli, err := getLinkFullInfo(lid)
		if err == nil {
			printLink(fli, true)
		}
	}

	return nil
}

func gWalkRoutes(fd, bd uint, verbose int) error {
	gWalkLoad()
	id := gWalkData.GetByPath("id").AsStringDefault("root")

	// Print routes as a tree -----------------------------------------------------------
	type vxe struct {
		id    string
		depth uint
	}

	tree := treeprint.New()
	tree.SetValue(id)

	visitedVerticesCache := map[string]fullVertexInfo{}
	visitedLinksCache := map[string]fullLinkInfo{}

	processTree := func(t treeprint.Tree, depth uint, backward bool) {
		processTreeLink := func(lid linkId, currentTree treeprint.Tree) (string, treeprint.Tree) {
			fli, ok := visitedLinksCache[lid.asStr()]
			if !ok {
				if i, err := getLinkFullInfo(lid); err == nil {
					fli = i
					visitedLinksCache[lid.asStr()] = fli
				} else {
					return "", nil
				}
			}

			targetId := fli.to
			if backward {
				targetId = fli.id.from
			}

			linkMeta := ""
			if verbose >= 1 {
				if verbose >= 2 {
					if len(fli.tags) > 0 {
						linkMeta = fmt.Sprintf("%s: %s | #%v", fli.id.name, fli.tp, strings.Join(fli.tags, " #"))
						return targetId, currentTree.AddMetaBranch(linkMeta, targetId)
					}
				}
				linkMeta = fmt.Sprintf("%s: %s", fli.id.name, fli.tp)
				return targetId, currentTree.AddMetaBranch(linkMeta, targetId)
			}
			linkMeta = fmt.Sprintf("%s", fli.id.name)
			return targetId, currentTree.AddMetaBranch(linkMeta, targetId)
		}

		stack := []vxe{{id, 0}}
		treeStack := []treeprint.Tree{t}
		for len(stack) > 0 {
			v := stack[0]
			stack = stack[1:]

			currentTree := treeStack[0]
			treeStack = treeStack[1:]

			if v.depth >= depth {
				continue
			}

			fvi, ok := visitedVerticesCache[v.id]
			if !ok {
				if i, err := getVertexFullInfo(v.id); err == nil {
					fvi = i
					visitedVerticesCache[v.id] = fvi
				} else {
					fmt.Printf("Cannot get vertex info: id=%s\n", fvi.id)
					continue
				}
			}

			links2Process := fvi.outLinks
			if backward {
				links2Process = fvi.inLinks
			}
			for _, lid := range links2Process {
				nextVertexId, nextTree := processTreeLink(lid, currentTree)
				if nextTree == nil {
					fmt.Printf("Cannot get link info: from=%s, name=%s\n", lid.from, lid.name)
					continue
				}
				stack = append(stack, vxe{nextVertexId, v.depth + 1})
				treeStack = append(treeStack, nextTree)
			}
		}
	}
	if fd > 0 {
		outs := tree.AddMetaBranch(fmt.Sprintf("depth=%d", fd), "OUT")
		processTree(outs, fd, false)
	}
	if bd > 0 {
		ins := tree.AddMetaBranch(fmt.Sprintf("depth=%d", bd), "IN")
		processTree(ins, bd, true)
	}
	fmt.Println(tree.String())
	// ----------------------------------------------------------------------------------

	return nil
}

func gWalkQuery(query string) error {
	const prefixIndent = "  "

	gWalkLoad()

	result, err := dbClient.Query.JPGQLCtraQuery(gWalkData.GetByPath("id").AsStringDefault("root"), query)
	if err != nil {
		return err
	}

	fmt.Println("Result:", strings.Join(result, ", "))

	return nil
}
