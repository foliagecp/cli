package main

import (
	"fmt"
	"os"

	"github.com/foliagecp/easyjson"
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

func gWalkQuery(algorithm string, query string) error {
	gWalkLoad()
	payload := easyjson.NewJSONObjectWithKeyValue("jpgql_query", easyjson.NewJSON(query))
	j, err := natsRequest(fmt.Sprintf("functions.graph.api.query.jpgql.%s", algorithm), gWalkData.GetByPath("id").AsStringDefault("root"), &payload, nil)
	if err != nil {
		return err
	}
	fmt.Println(JSONStrPrettyStringAnyway(j.GetByPath("result").ToString()))
	return nil
}
