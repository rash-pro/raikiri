package app

import (
	"html"
	"regexp"
)

var tagRe = regexp.MustCompile(`<[^>]*>`)

func sanitizeText(s string) string {
	return html.EscapeString(tagRe.ReplaceAllString(s, ""))
}
