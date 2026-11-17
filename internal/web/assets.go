package web

import (
	_ "embed"
	"net/http"
)

// picoVersion pins the embedded Pico CSS (https://picocss.com, MIT). The file
// is the unchanged pico.min.css of the npm release @picocss/pico@<picoVersion>
// (jsDelivr), kept in assets/. To update: replace the file, rename it and
// change this constant, the version in the path is the cache key.
const picoVersion = "2.1.1"

//go:embed assets/pico-2.1.1.min.css
var picoCSS []byte

// picoPath is the public route of the Pico file, without the ingress base.
const picoPath = "/assets/pico-" + picoVersion + ".min.css"

// handlePicoCSS serves the embedded Pico file. The version is part of the
// path, so the browser may keep it for good.
func handlePicoCSS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(picoCSS)
	}
}
