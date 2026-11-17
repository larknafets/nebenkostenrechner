package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const picoPfad = "/assets/pico-2.1.1.min.css"

// The Pico file is public (no login, like the health route), served as CSS
// with a long cache because the version is part of the path, also behind
// the ingress path (HA strips the prefix before the request gets here).
func TestPico_RouteOeffentlich(t *testing.T) {
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	for _, ingress := range []string{"", "/api/hassio_ingress/abc"} {
		req := httptest.NewRequest(http.MethodGet, picoPfad, nil)
		if ingress != "" {
			req.Header.Set("X-Ingress-Path", ingress)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("ingress %q: status %d, want 200", ingress, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
			t.Errorf("Content-Type %q, want text/css", ct)
		}
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=") || !strings.Contains(cc, "immutable") {
			t.Errorf("Cache-Control %q, want a long immutable cache", cc)
		}
		if !strings.Contains(w.Body.String(), "Pico CSS") {
			t.Errorf("body is not the Pico file")
		}
	}
}

// Every page links Pico first and the own CSS after it, with the base prefix.
// The login dialog lives in every page for a visitor, so it is styled too.
func TestPico_SeitenVerlinkenDieDatei(t *testing.T) {
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	for _, base := range []string{"", "/api/hassio_ingress/abc"} {
		req := httptest.NewRequest(http.MethodGet, "/berechnungslogik", nil)
		if base != "" {
			req.Header.Set("X-Ingress-Path", base)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		body := w.Body.String()
		link := `<link rel="stylesheet" href="` + base + picoPfad + `">`
		i := strings.Index(body, link)
		if i < 0 {
			t.Fatalf("base %q: no %s in the page", base, link)
		}
		if j := strings.Index(body, "<style>"); j < i {
			t.Errorf("base %q: own <style> comes before Pico", base)
		}
		if !strings.Contains(body, `id="login-overlay"`) {
			t.Errorf("base %q: no login dialog for the visitor", base)
		}
	}
}
