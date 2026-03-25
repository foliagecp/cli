package main

import (
	"os/user"
	"path/filepath"
	"strings"

	"github.com/foliagecp/easyjson"
)

func JSONStrPrettyString(j *easyjson.JSON, everyLineIndent int, innerIndent int) string {
	s := colorJSON(j.Value, innerIndent, 0)
	pi := strings.Repeat(" ", everyLineIndent)
	res := strings.ReplaceAll(s, "\n", "\n"+pi)
	return res
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
