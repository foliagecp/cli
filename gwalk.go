package main

import (
	"fmt"
	"html"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/foliagecp/easyjson"
	sfMediators "github.com/foliagecp/sdk/statefun/mediator"
	sfp "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
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

	// outFull/inFull carry the link target and type harvested from the same
	// vertex read, so the TUI does not have to issue one read per link just to
	// learn them. Body and tags are deliberately absent — nothing in the list
	// view shows them, and they are fetched lazily when an editor opens.
	//
	// Empty when the server did not return the structured form; callers must
	// fall back to reading each link individually.
	outFull []fullLinkInfo
	inFull  []fullLinkInfo
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

	if resErr = initDBClient(); resErr != nil {
		return
	}
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

	if resErr = initDBClient(); resErr != nil {
		return
	}
	// details_v2 returns links.out as [{to,name,type}] instead of three
	// parallel arrays. That matters for more than tidiness: the legacy form
	// appends to `names` but can skip `types`/`ids` for a broken target, so the
	// arrays desync and cannot be zipped safely. The structured form carries
	// each link's target and type with the link itself.
	data, err := dbClient.Graph.VertexReadDetailsV2(vertexId)
	if err != nil {
		resErr = err
		return
	}

	fvi.body = data.GetByPath("body").GetPtr()

	outLinks := data.GetByPath("links.out")
	if outLinks.IsArray() {
		for i := 0; i < outLinks.ArraySize(); i++ {
			ol := outLinks.ArrayElement(i)
			name := ol.GetByPath("name").AsStringDefault("")
			fvi.outLinks = append(fvi.outLinks, linkId{vertexId, name})
			fvi.outFull = append(fvi.outFull, fullLinkInfo{
				id: linkId{vertexId, name},
				to: ol.GetByPath("to").AsStringDefault(""),
				tp: ol.GetByPath("type").AsStringDefault(""),
			})
		}
	} else if arr, ok := data.GetByPath("links.out.names").AsArrayString(); ok {
		// Older runtime that ignored details_v2: names only, so the targets
		// and types still have to be read one link at a time.
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
		fvi.inFull = append(fvi.inFull, fullLinkInfo{
			id: linkId{from, linkName},
			to: vertexId,
			tp: inLink.GetByPath("type").AsStringDefault(""),
		})
	}

	return
}

func gWalkInspect(prettyPrint bool, allData bool) error {
	const prefixIndent = "  "
	system.MsgOnErrorReturn(gWalkLoad())

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

	if allData {
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
	}

	return nil
}

func gWalkRoutes(fd, bd uint, verbose int) error {
	system.MsgOnErrorReturn(gWalkLoad())
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
			linkMeta = fli.id.name
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

func gWalkGetGraph(format string, root string, depth int, excludeVertex, excludeEdge []string) (string, error) {
	// Graphml json body patch ----------------------------------------------------------
	// normalizeGraphMLJSONBodies finds all <data key="bdj"> ... </data> entries,
	// parses their content as JSON, normalizes it (arrays order, numeric unification),
	// and writes it back (XML-escaped). XML bodies (key="bdx") are untouched.
	var reBDJ = regexp.MustCompile(`(?s)(<data\s+key=['"]bdj['"]>)(.*?)(</data>)`)

	normalizeGraphMLJSONBodies := func(graphml string) string {
		return reBDJ.ReplaceAllStringFunc(graphml, func(m string) string {
			sub := reBDJ.FindStringSubmatch(m)
			if len(sub) != 4 {
				return m
			}
			open, bodyRaw, close := sub[1], sub[2], sub[3]

			// Optional CDATA wrapper support
			body := bodyRaw
			if len(body) >= 12 && strings.HasPrefix(body, "<![CDATA[") && strings.HasSuffix(body, "]]>") {
				body = body[len("<![CDATA[") : len(body)-len("]]>")]
			}

			// Unescape XML entities (&#34;, &amp;, &lt;, &gt;, …) to get plain JSON text
			body = html.UnescapeString(strings.TrimSpace(body))
			j, ok := easyjson.JSONFromString(body)
			if !ok {
				// Not a JSON body — leave as is
				return m
			}

			// Normalize using the easyjson.Normalize you added
			j.Normalize()
			norm := j.ToString()

			// Escape back for XML text node (quotes don't need escaping in element text)
			escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(norm)
			return open + escaped + close
		})
	}

	originalFormat := format
	// ----------------------------------------------------------------------------------

	system.MsgOnErrorReturn(gWalkLoad())

	if err := initDBClient(); err != nil {
		return "", err
	}
	payload := easyjson.NewJSONObjectWithKeyValue("depth", easyjson.NewJSON(depth))
	if format == "graphml_json2xml" {
		format = "graphml"
		payload.SetByPath("json2xml", easyjson.NewJSON(true))
	}
	payload.SetByPath("format", easyjson.NewJSON(format))
	payload.SetByPath("delivery", easyjson.NewJSON("auto"))
	// Build exclude arrays if provided
	if len(excludeVertex) > 0 {
		arr := easyjson.NewJSONArray().GetPtr()
		for _, v := range excludeVertex {
			arr.AddToArray(easyjson.NewJSON(v))
		}
		payload.SetByPath("exclude.vertex", *arr)
	}
	if len(excludeEdge) > 0 {
		arr := easyjson.NewJSONArray().GetPtr()
		for _, v := range excludeEdge {
			arr.AddToArray(easyjson.NewJSON(v))
		}
		payload.SetByPath("exclude.edge", *arr)
	}

	om := sfMediators.OpMsgFromSfReply(
		dbClient.Request(sfp.AutoRequestSelect, "functions.graph.api.object.debug.print.graph", root, &payload, nil),
	)
	if om.Status != sfMediators.SYNC_OP_STATUS_OK {
		return "", fmt.Errorf("%s", om.Details)
	}

	delivery := om.Data.GetByPath("delivery").AsStringDefault("inline")

	var out string

	switch delivery {
	case "chunks":
		sessionID := om.Data.GetByPath("session_id").AsStringDefault("")
		vertexID := om.Data.GetByPath("vertex_id").AsStringDefault(root)
		totalChunks := int(om.Data.GetByPath("total_chunks").AsNumericDefault(0))
		totalBytes := int64(om.Data.GetByPath("total_bytes").AsNumericDefault(0))

		if totalBytes < 0 || totalBytes > math.MaxInt {
			return "", fmt.Errorf("export too large for in-memory assembly: %d bytes", totalBytes)
		}

		var buf strings.Builder
		buf.Grow(int(totalBytes))

		for i := 0; i < totalChunks; i++ {
			chunkPayload := easyjson.NewJSONObject()
			chunkPayload.SetByPath("export_action", easyjson.NewJSON("get_chunk"))
			chunkPayload.SetByPath("session_id", easyjson.NewJSON(sessionID))
			chunkPayload.SetByPath("chunk_index", easyjson.NewJSON(i))

			chunkOM := sfMediators.OpMsgFromSfReply(
				dbClient.Request(sfp.AutoRequestSelect, "functions.graph.api.object.debug.print.graph", vertexID, &chunkPayload, nil),
			)
			if chunkOM.Status != sfMediators.SYNC_OP_STATUS_OK {
				return "", fmt.Errorf("chunk %d/%d: %s", i, totalChunks, chunkOM.Details)
			}
			buf.WriteString(chunkOM.Data.GetByPath("data").AsStringDefault(""))
		}

		// Best-effort cleanup: TTL on the server handles abandoned sessions
		finishPayload := easyjson.NewJSONObject()
		finishPayload.SetByPath("export_action", easyjson.NewJSON("finish_session"))
		finishPayload.SetByPath("session_id", easyjson.NewJSON(sessionID))
		_, _ = dbClient.Request(sfp.AutoRequestSelect, "functions.graph.api.object.debug.print.graph", vertexID, &finishPayload, nil)

		out = buf.String()
		// No Normalize() call needed here: buf contains the raw string content assembled from
		// chunk data fields; it is not a JSON-wrapped leaf, so there is nothing to normalize.

	default: // "inline" or old server (no delivery field)
		fileJSON := om.Data.GetByPath("file").GetPtr()
		fileJSON.Normalize() // no-op on a JSON string leaf, kept for parity with historic behavior
		out = fileJSON.AsStringDefault("")
	}

	// Graphml json body patch ----------------------------------------------------------
	// Applies only to "graphml" format. "graphml_json2xml" exports bodies as XML (bdx keys),
	// not JSON (bdj keys), so there is nothing for the regex to normalize in that mode.
	if originalFormat == "graphml" {
		out = normalizeGraphMLJSONBodies(out)
	}
	// ----------------------------------------------------------------------------------

	return out, nil
}

func gWalkSetGraph(format string, root string, data string) error {
	system.MsgOnErrorReturn(gWalkLoad())

	if err := initDBClient(); err != nil {
		return err
	}
	payload := easyjson.NewJSONObjectWithKeyValue("source", easyjson.NewJSON("payload"))
	payload.SetByPath("format", easyjson.NewJSON(format))
	payload.SetByPath("data", easyjson.NewJSON(data))
	om := sfMediators.OpMsgFromSfReply(
		dbClient.Request(sfp.AutoRequestSelect, "functions.graph.api.import", root, &payload, nil, 300*time.Second),
	)
	if om.Status != sfMediators.SYNC_OP_STATUS_OK {
		return fmt.Errorf("%s", om.Details)
	}
	return nil
}

func gWalkPrintGraph(format string, depth int, raw bool, excludeVertex, excludeEdge []string) error {
	system.MsgOnErrorReturn(gWalkLoad())
	root := gWalkData.GetByPath("id").AsStringDefault("root")

	dotFileStr, err := gWalkGetGraph(format, root, depth, excludeVertex, excludeEdge)
	if err != nil {
		return err
	}

	if !raw {
		fmt.Printf("Graph in %s format\n", format)
		fmt.Println("  From vertex:", root)
		fmt.Println("  Depth:", depth)

		fmt.Println()
		fmt.Println("Content")
	}
	fmt.Println(dotFileStr)

	return nil
}

func gWalkImportGraph(format string, graphData string) error {
	system.MsgOnErrorReturn(gWalkLoad())
	root := gWalkData.GetByPath("id").AsStringDefault("root")

	err := gWalkSetGraph(format, root, graphData)
	if err != nil {
		return err
	}

	return nil
}

func gWalkQuery(query string) error {
	system.MsgOnErrorReturn(gWalkLoad())

	if err := initDBClient(); err != nil {
		return err
	}
	result, err := dbClient.Query.JPGQLCtraQuery(gWalkData.GetByPath("id").AsStringDefault("root"), query)
	if err != nil {
		return err
	}

	fmt.Println("Result:", strings.Join(result, ", "))

	return nil
}
