// Package webassets embeds the phone control page's static files (web/)
// into the binary, so the Pi needs nothing on disk beyond the compiled
// executable to serve the control site.
package webassets

import (
	"embed"
	"io/fs"
)

//go:embed web
var raw embed.FS

// FS is the embedded web/ directory, rooted so that index.html, app.js and
// styles.css sit at its top level — matching the absolute paths
// (/styles.css, /app.js) that index.html itself references.
var FS = mustSub(raw, "web")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
