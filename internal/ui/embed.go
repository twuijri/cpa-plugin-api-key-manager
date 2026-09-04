package ui

import "embed"

//go:embed console.html app.js direct.js pricing.js picker.js i18n.js theme.js style.css picker.css theme.css
var files embed.FS

func Asset(name string) ([]byte, string, bool) {
	ct := ""
	switch name {
	case "console":
		name = "console.html"
		ct = "text/html; charset=utf-8"
	case "app.js", "direct.js", "pricing.js", "picker.js", "i18n.js", "theme.js":
		ct = "text/javascript; charset=utf-8"
	case "style.css", "picker.css", "theme.css":
		ct = "text/css; charset=utf-8"
	default:
		return nil, "", false
	}
	b, err := files.ReadFile(name)
	return b, ct, err == nil
}
