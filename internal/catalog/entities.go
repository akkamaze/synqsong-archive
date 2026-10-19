package catalog

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The Data API escapes titles; only these are decoded, so any other "&" text stays as written.
var (
	entity        = regexp.MustCompile(`&(#\d+|#x[0-9a-fA-F]+|[a-zA-Z]+);`)
	namedEntities = map[string]string{"amp": "&", "lt": "<", "gt": ">", "quot": `"`, "apos": "'"}
)

// cleanText decodes the Data API's escaping once, where the text enters: "&amp;amp;" becomes "&amp;".
func cleanText(s string) string {
	return strings.TrimSpace(entity.ReplaceAllStringFunc(s, decodeEntity))
}

func decodeEntity(match string) string {
	name := match[1 : len(match)-1]

	if plain, ok := namedEntities[name]; ok {
		return plain
	}

	digits, base := strings.TrimPrefix(name, "#"), 10
	if digits == name {
		return match
	}

	if hex, ok := strings.CutPrefix(digits, "x"); ok {
		digits, base = hex, 16
	}

	code, err := strconv.ParseInt(digits, base, 32)
	if err != nil || code == 0 || !utf8.ValidRune(rune(code)) {
		return match
	}

	return string(rune(code))
}
