// Command nebenkostenrechner runs the Nebenkostenrechner web server.
package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
	"github.com/larknafets/nebenkostenrechner/internal/web"
)

// legacyDBPath is the sole DB_PATH default before the HA add-on moved its
// data onto the addon_configs mount (Issue #TBD). migrateLegacyDBPath copies
// an existing DB found there once, so upgrading add-on installs that switch
// DB_PATH to /config don't appear to lose their data.
const legacyDBPath = "/data/nebenkosten.db"

func migrateLegacyDBPath(dbPath string) error {
	return migrateDBPathFrom(dbPath, legacyDBPath)
}

func migrateDBPathFrom(dbPath, oldPath string) error {
	if dbPath == oldPath {
		return nil
	}
	if _, err := os.Stat(dbPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	src, err := os.Open(oldPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer src.Close()

	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	dst, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	log.Printf("migrated database from legacy path %s to %s", oldPath, dbPath)
	return nil
}

// version and buildDate are set via -ldflags at build time (Docker build
// args and GoReleaser, Ticket #48) - both stay empty for a local `go run`,
// which hides the Dashboard's version badge entirely.
var (
	version   = ""
	buildDate = ""
)

// demoDBPath derives demo mode's own DB file (Issue #120) - lives in the
// same directory as the real DB, so it survives restarts on the same
// persistent volume (e.g. /data, HA add-on addon_configs).
func demoDBPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "demo.db")
}

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/nebenkosten.db"
	}

	if err := migrateLegacyDBPath(dbPath); err != nil {
		log.Fatalf("migrate legacy db path: %v", err)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// demoDB is a completely separate database for demo mode (Issue #120) -
	// created automatically on the very first start (store.Open sets up
	// schema+seed just like for db) and, if still empty, immediately filled
	// with the 39-month test dataset. This only covers the very first start,
	// before anyone has ever logged in as "demo" - every demo login itself
	// unconditionally resets the demo DB anyway (Issue #121,
	// store.ResetDemoData in handleLogin), so this startup fill-in is just
	// the fallback for the period before that.
	demoDB, err := store.Open(demoDBPath(dbPath))
	if err != nil {
		log.Fatalf("open demo store: %v", err)
	}
	defer demoDB.Close()
	demoPeriods, err := store.AllPeriods(demoDB)
	if err != nil {
		log.Fatalf("demo store: %v", err)
	}
	if len(demoPeriods) == 0 {
		if err := store.SeedDemoData(demoDB, time.Now()); err != nil {
			log.Fatalf("seed demo data: %v", err)
		}
	}

	mux := web.NewMux(db, demoDB, version, buildDate)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// widgetAddr serves NewWidgetMux's 2 read-only HA widget routes
	// (Issue #77 ff.) on a 2nd, ingress-free port - meant for a Lovelace
	// "Webpage card" iframe, which can't reliably embed HA ingress sessions.
	// Always runs alongside the main server, own default port analogous to
	// LISTEN_ADDR.
	widgetAddr := os.Getenv("WIDGET_LISTEN_ADDR")
	if widgetAddr == "" {
		widgetAddr = ":8081"
	}
	go func() {
		log.Printf("widget routes listening on %s", widgetAddr)
		if err := http.ListenAndServe(widgetAddr, web.NewWidgetMux(db)); err != nil {
			log.Fatalf("widget serve: %v", err)
		}
	}()

	log.Printf("listening on %s (db: %s)", addr, dbPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
