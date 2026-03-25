package main

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
)

func colorJSON(v any, indent int, depth int) string {
	pad := strings.Repeat(" ", indent*depth)
	childPad := strings.Repeat(" ", indent*(depth+1))

	switch val := v.(type) {
	case map[string]any:
		if len(val) == 0 {
			return "{}"
		}
		var b bytes.Buffer
		b.WriteString("{\n")
		i := 0
		for k, child := range val {
			b.WriteString(childPad)
			fmt.Fprintf(&b, "%s%s\"%s\"%s: ", ansiBold, ansiBlue, k, ansiReset)
			b.WriteString(colorJSON(child, indent, depth+1))
			if i < len(val)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
			i++
		}
		b.WriteString(pad + "}")
		return b.String()

	case []any:
		if len(val) == 0 {
			return "[]"
		}
		var b bytes.Buffer
		b.WriteString("[\n")
		for i, child := range val {
			b.WriteString(childPad)
			b.WriteString(colorJSON(child, indent, depth+1))
			if i < len(val)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(pad + "]")
		return b.String()

	case string:
		return fmt.Sprintf("%s\"%s\"%s", ansiGreen, val, ansiReset)

	case float64:
		return fmt.Sprintf("%s%g%s", ansiCyan, val, ansiReset)

	case bool:
		return fmt.Sprintf("%s%t%s", ansiYellow, val, ansiReset)

	case nil:
		return fmt.Sprintf("%snull%s", ansiYellow, ansiReset)

	default:
		return fmt.Sprintf("%v", val)
	}
}
