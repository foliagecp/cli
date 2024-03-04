package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/sdk/statefun"
	sfMediators "github.com/foliagecp/sdk/statefun/mediator"
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

func natsRequest(targetTypename string, targetID string, payload *easyjson.JSON, options *easyjson.JSON) (*sfMediators.OpMsg, error) {
	targetDomain := NatsHubDomain
	tokens := strings.Split(targetID, statefun.ObjectIDDomainSeparator)
	if len(tokens) == 2 {
		targetDomain = tokens[0]
	}

	resp, err := nc.Request(
		fmt.Sprintf("request.%s.%s.%s", targetDomain, targetTypename, targetID),
		buildNatsData("cli", "cli", payload, options),
		time.Duration(NatsRequestTimeoutSec)*time.Second,
	)
	if err == nil {
		if j, ok := easyjson.JSONFromBytes(resp.Data); ok {
			msg := sfMediators.OpMsgFromSfReply(&j, nil)
			return &msg, nil
		}
		return nil, fmt.Errorf("response from function typename \"%s\" with id \"%s\" is not a json", targetTypename, targetID)
	}
	return nil, err
}
