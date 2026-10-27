package web

import (
	"log"
	"net/http"
	"strings"
)

// keineBerechtigungSeite answers a logged-in Mieter who asked for a page
// reserved for the Vermieter or not granted to him: 403 with a link that opens
// the login as Vermieter and leads back to the wanted page.
func (z *Zugang) keineBerechtigungSeite(w http.ResponseWriter, r *http.Request, wanted, titel, text string) {
	if wanted == "" || !strings.HasPrefix(wanted, "/") || strings.HasPrefix(wanted, "//") {
		wanted = "/dashboard"
	}
	data := struct {
		navData
		Aktuell string
		Next    string
		Titel   string
		Text    string
	}{navData: z.NavData(r), Next: wanted, Titel: titel, Text: text}
	w.WriteHeader(http.StatusForbidden)
	if err := keineBerechtigungTemplate.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("keine berechtigung: %v", err)
	}
}
