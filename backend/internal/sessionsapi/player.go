package sessionsapi

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// The asciinema player bundle is vendored and embedded so exported recordings
// play fully offline (no CDN / network dependency).
//
//go:embed player/asciinema-player.min.js
var playerJS string

//go:embed player/asciinema-player.css
var playerCSS string

// renderPlayerHTML produces a single self-contained HTML document that plays the
// given asciicast offline: the player CSS + JS and the recording are all inlined.
func renderPlayerHTML(title, cast string) string {
	castJSON, _ := json.Marshal(cast) // HTML-safe (escapes <, >, &)
	// Guard against a literal </script> inside the bundle breaking the HTML parser.
	js := strings.ReplaceAll(playerJS, "</script", "<\\/script")

	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">\n")
	b.WriteString("<title>" + escapeHTML(title) + "</title>\n<style>\n")
	b.WriteString(playerCSS)
	b.WriteString("\nhtml,body{margin:0;background:#0d1117;color:#c9d1d9;font-family:system-ui,sans-serif}")
	b.WriteString("header{padding:10px 16px;font-size:14px;border-bottom:1px solid #30363d}#player{padding:8px}")
	b.WriteString("\n</style></head><body>\n")
	b.WriteString("<header>" + escapeHTML(title) + " — plays offline</header>\n")
	b.WriteString("<div id=\"player\"></div>\n<script>\n")
	b.WriteString(js)
	b.WriteString("\n</script>\n<script>\n")
	b.WriteString("var cast = " + string(castJSON) + ";\n")
	b.WriteString("var blob = new Blob([cast], {type: \"application/x-asciicast\"});\n")
	b.WriteString("AsciinemaPlayer.create(URL.createObjectURL(blob), document.getElementById(\"player\"), {fit: \"width\", terminalFontSize: \"small\"});\n")
	b.WriteString("</script>\n</body></html>\n")
	return b.String()
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}
