package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseSemver(t *testing.T) {
	cases := []struct {
		raw                 string
		major, minor, patch int
		ok                  bool
	}{
		{"v1.2.3", 1, 2, 3, true},
		{"1.2.3", 1, 2, 3, true},
		{"v0.7.0", 0, 7, 0, true},
		{"dev", 0, 0, 0, false},
		{"", 0, 0, 0, false},
		{"v1.2", 0, 0, 0, false},
		{"v1.2.3.4", 0, 0, 0, false},
		{"v1.2.x", 0, 0, 0, false},
	}
	for _, c := range cases {
		major, minor, patch, ok := parseSemver(c.raw)
		if ok != c.ok || (ok && (major != c.major || minor != c.minor || patch != c.patch)) {
			t.Errorf("parseSemver(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)", c.raw, major, minor, patch, ok, c.major, c.minor, c.patch, c.ok)
		}
	}
}

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.8.0", "v0.7.0", true},
		{"v1.0.0", "v0.7.0", true},
		{"v0.7.1", "v0.7.0", true},
		{"v0.7.0", "v0.7.0", false},
		{"v0.6.0", "v0.7.0", false},
		{"v0.8.0", "dev", false},
		{"garbage", "v0.7.0", false},
		{"v0.8.0", "", false},
		{"v0.8.0", "nightly.20261001-e3c101d", false}, // channels never mix
		{"nightly.20261020-aaaaaaa", "nightly.20261019-bbbbbbb", true},
		{"nightly.20261019-aaaaaaa", "nightly.20261019-bbbbbbb", false}, // same day: no order
		{"nightly.20261018-aaaaaaa", "nightly.20261019-bbbbbbb", false},
		{"nightly.20261020-aaaaaaa", "v0.17.0", false},
		{"nightly.20261020-aaaaaaa", "dev", false},
	}
	for _, c := range cases {
		if got := isNewerVersion(c.latest, c.current); got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestFetchLatestReleaseFrom(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"tag_name":"v0.8.0","name":"v0.8.0"}`))
		}))
		defer srv.Close()

		tag, err := fetchLatestReleaseFrom(srv.URL, srv.Client())
		if err != nil {
			t.Fatalf("fetchLatestReleaseFrom: %v", err)
		}
		if tag != "v0.8.0" {
			t.Errorf("tag = %q, want v0.8.0", tag)
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		if _, err := fetchLatestReleaseFrom(srv.URL, srv.Client()); err == nil {
			t.Error("want error for 404, got nil")
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()

		if _, err := fetchLatestReleaseFrom(srv.URL, srv.Client()); err == nil {
			t.Error("want error for malformed JSON, got nil")
		}
	})
}

func TestParseNightly(t *testing.T) {
	cases := []struct {
		raw, date, sha string
		ok             bool
	}{
		{"nightly.20261019-46924b3", "20261019", "46924b3", true},
		{" nightly.20261019-46924b3 ", "20261019", "46924b3", true},
		{"v0.17.0", "", "", false},
		{"nightly", "", "", false},
		{"nightly.2026101-46924b3", "", "", false},
		{"nightly.20261019-46924b", "", "", false},
		{"nightly.20261019-46924bz", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		date, sha, ok := parseNightly(c.raw)
		if ok != c.ok || date != c.date || sha != c.sha {
			t.Errorf("parseNightly(%q) = (%q,%q,%v), want (%q,%q,%v)", c.raw, date, sha, ok, c.date, c.sha, c.ok)
		}
	}
}

func TestUpdateURL(t *testing.T) {
	if got, want := updateURL("v0.17.0"), "https://github.com/larknafets/nebenkostenrechner/releases/tag/v0.17.0"; got != want {
		t.Errorf("updateURL(release) = %q, want %q", got, want)
	}
	if got, want := updateURL("nightly.20261019-46924b3"), "https://github.com/larknafets/nebenkostenrechner/commit/46924b3"; got != want {
		t.Errorf("updateURL(nightly) = %q, want %q", got, want)
	}
}

func TestFetchLatestNightlyFrom(t *testing.T) {
	serve := func(status int, body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(body))
		}))
	}

	t.Run("success", func(t *testing.T) {
		srv := serve(http.StatusOK, "name: Nebenkosten (Nightly)\nversion: \"nightly.20261019-46924b3\"\nslug: nebenkostenrechner-nightly\n")
		defer srv.Close()
		v, err := fetchLatestNightlyFrom(srv.URL, srv.Client())
		if err != nil || v != "nightly.20261019-46924b3" {
			t.Errorf("fetchLatestNightlyFrom = (%q, %v), want nightly.20261019-46924b3", v, err)
		}
	})

	for name, c := range map[string]struct {
		status int
		body   string
	}{
		"non-200 status":  {http.StatusNotFound, "404: Not Found"},
		"no version line": {http.StatusOK, "name: x\nslug: y\n"},
		"not a nightly":   {http.StatusOK, "version: \"0.17.0\"\n"},
		"empty body":      {http.StatusOK, ""},
	} {
		t.Run(name, func(t *testing.T) {
			srv := serve(c.status, c.body)
			defer srv.Close()
			if _, err := fetchLatestNightlyFrom(srv.URL, srv.Client()); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
