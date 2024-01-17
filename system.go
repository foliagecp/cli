package main

import (
	"os/user"
	"path/filepath"
	"strings"

	"github.com/TylerBrock/colorjson"
	"github.com/foliagecp/easyjson"
)

func JSONStrPrettyStringAnyway(j *easyjson.JSON, everyLineIndent int, innerIndent int) string {
	ppStr, err := JSONStrPrettyString(j, everyLineIndent, innerIndent)
	if err != nil {
		return ppStr
	}
	return ppStr
}

func JSONStrPrettyString(j *easyjson.JSON, everyLineIndent int, innerIndent int) (string, error) {
	f := colorjson.NewFormatter()
	f.Indent = innerIndent
	s, err := f.Marshal(j.Value)
	if err != nil {
		return "", err
	}
	pi := ""
	for i := 0; i < everyLineIndent; i++ {
		pi += " "
	}
	res := strings.ReplaceAll(string(s), "\n", "\n"+pi)
	return res, nil
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
