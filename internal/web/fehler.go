package web

import "net/http"

// fehlerFeldHeader names the form field a 400 response is about. The
// Fehler-Overlay (layout.html) marks that field and puts the focus back on
// it after the dialog closes. Optional: without it the overlay shows only
// the message.
const fehlerFeldHeader = "X-Fehler-Feld"

// badRequestFeld answers 400 with a user-facing message and the name of the
// offending form field.
func badRequestFeld(w http.ResponseWriter, msg, feld string) {
	w.Header().Set(fehlerFeldHeader, feld)
	http.Error(w, msg, http.StatusBadRequest)
}
