package main

import (
	"fmt"
	"time"

	"github.com/foliagecp/easyjson"
)

func buildNatsData(callerTypename string, callerID string, payload *easyjson.JSON, options *easyjson.JSON) []byte {
	data := easyjson.NewJSONObject()
	data.SetByPath("caller_typename", easyjson.NewJSON(callerTypename))
	data.SetByPath("caller_id", easyjson.NewJSON(callerID))
	if payload != nil {
		data.SetByPath("payload", *payload)
	}
	if options != nil {
		data.SetByPath("options", *options)
	}
	return data.ToBytes()
}

func natsRequest(targetTypename string, targetID string, payload *easyjson.JSON, options *easyjson.JSON) (*easyjson.JSON, error) {
	resp, err := nc.Request(
		fmt.Sprintf("service.%s.%s", targetTypename, targetID),
		buildNatsData("cli", "cli", payload, options),
		time.Duration(NatsRequestTimeoutSec)*time.Second,
	)
	if err == nil {
		if j, ok := easyjson.JSONFromBytes(resp.Data); ok {
			return &j, nil
		}
		return nil, fmt.Errorf("response from function typename \"%s\" with id \"%s\" is not a json", targetTypename, targetID)
	}
	return nil, err
}
