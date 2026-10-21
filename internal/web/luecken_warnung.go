package web

import (
	_ "embed"
	"html/template"
)

// lueckenWarnungJS is the gap hint of the reading wizard, kept in its own file
// so that luecken_warnung_test.go can run exactly the code the browser gets.
//
//go:embed templates/luecken_warnung.js
var lueckenWarnungJS string

func lueckenWarnungScript() template.JS { return template.JS(lueckenWarnungJS) }
