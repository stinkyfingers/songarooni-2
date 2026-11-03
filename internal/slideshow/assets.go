package slideshow

import (
	"embed"
	"io/fs"
)

//go:embed web
var rawAssets embed.FS

// webAssets is the embedded web/ directory, rooted so that index.html,
// app.js and styles.css sit at its top level — matching the absolute paths
// (/styles.css, /app.js) that index.html itself references. Same pattern
// internal/control-panel uses for its own static assets.
var webAssets = mustSub(rawAssets, "web")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
