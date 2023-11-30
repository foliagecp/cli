package main

import (
	"bytes"
	"encoding/json"
	"os/user"
	"path/filepath"
	"strings"
)

func JSONStrPrettyStringAnyway(str, prefix, indent string) string {
	ppStr, err := JSONStrPrettyString(str, prefix, indent)
	if err != nil {
		return str
	}
	return ppStr
}

func JSONStrPrettyString(str, prefix, indent string) (string, error) {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, []byte(str), prefix, indent); err != nil {
		return "", err
	}
	return prettyJSON.String(), nil
}

func expandFileName(fileName string) (string, error) {
	if strings.HasPrefix(fileName, "~") {
		usr, err := user.Current()
		if err != nil {
			return "", err
		}
		homeDir := usr.HomeDir
		fileName = filepath.Join(homeDir, fileName[1:])
	}

	expandedPath, err := filepath.Abs(fileName)
	if err != nil {
		return "", err
	}

	return expandedPath, nil
}
