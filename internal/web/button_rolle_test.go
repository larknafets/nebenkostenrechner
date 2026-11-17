package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// Pico only styles a link as a button when it carries role="button".
var linkImButtonLook = regexp.MustCompile(`<a [^>]*class="button[^"]*"[^>]*>`)

func TestLinksImButtonLookTragenRoleButton(t *testing.T) {
	seiten := []struct {
		pfad string
		h    http.HandlerFunc
	}{
		{"/fixkosten/neu", handleFixkostenForm(newZugang("", nil))},
		{"/fixkosten", handleFixkostenListe(newZugang("", nil))},
		{"/ablesungen", handleAblesungenListe(newZugang("", nil))},
	}
	for _, s := range seiten {
		body := getPage(t, s.h, s.pfad)
		links := linkImButtonLook.FindAllString(body, -1)
		if len(links) == 0 {
			t.Errorf("%s: kein Link im Button-Look gefunden", s.pfad)
		}
		for _, l := range links {
			if !strings.Contains(l, `role="button"`) {
				t.Errorf("%s: role=\"button\" fehlt: %s", s.pfad, l)
			}
		}
	}
}
