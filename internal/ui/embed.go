package ui

import "embed"

//go:embed console.html app.js direct.js style.css
var files embed.FS

func Asset(name string) ([]byte, string, bool) {
	ct := ""
	switch name {
	case "console":
		name = "console.html"
		ct = "text/html; charset=utf-8"
	case "app.js", "direct.js":
		ct = "text/javascript; charset=utf-8"
	case "style.css":
		ct = "text/css; charset=utf-8"
	default:
		return nil, "", false
	}
	b, err := files.ReadFile(name)
	return b, ct, err == nil
}
