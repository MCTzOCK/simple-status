// Package web embeds the static status page assets so the binary serves
// them without any external files.
package web

import "embed"

// Files holds the status page assets (index.html, app.js, style.css, favicon.svg).
//
//go:embed index.html app.js style.css favicon.svg
var Files embed.FS
