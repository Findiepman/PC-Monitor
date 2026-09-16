// Package webui embeds the built frontend. `npm run build` in web/ writes
// into dist/.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
