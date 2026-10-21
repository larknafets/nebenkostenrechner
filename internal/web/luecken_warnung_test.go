package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLueckenWarnung runs the browser code of the gap hint (Issue #257) under
// Node in several time zones. The hint compares calendar days, so the result
// must be the same everywhere: exactly one month used to warn east of UTC.
func TestLueckenWarnung(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}

	cases := []struct{ vorher, neu, want string }{
		{"2026-09-01", "2026-10-01", ""}, // exactly one month, the reported case
		{"2026-09-01", "2026-10-02", "⚠ Verbrauch über 1 Monat (letzte Ablesung: 01.09.2026)"},
		{"2026-09-01", "2026-11-01", "⚠ Verbrauch über 2 Monate (letzte Ablesung: 01.09.2026)"},
		{"2025-12-15", "2026-01-15", ""}, // year change
		{"2025-12-15", "2026-01-16", "⚠ Verbrauch über 1,1 Monate (letzte Ablesung: 15.12.2025)"},
		{"2026-01-31", "2026-02-28", ""}, // end of month is clamped
		{"2026-01-31", "2026-03-01", "⚠ Verbrauch über 1 Monat (letzte Ablesung: 31.01.2026)"},
		{"2024-01-31", "2024-02-29", ""}, // leap year
		{"2026-09-01", "2026-08-01", ""}, // new date before the last reading
		{"", "2026-10-01", ""},
		{"2026-09-01", "", ""},
		{"x", "y", ""},
	}
	args := make([][2]string, len(cases))
	for i, c := range cases {
		args[i] = [2]string{c.vorher, c.neu}
	}
	cj, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}

	script := lueckenWarnungJS + "\nconsole.log(JSON.stringify(" + string(cj) + ".map(([v, n]) => lueckenWarnung(v, n))));\n"
	file := filepath.Join(t.TempDir(), "luecken.js")
	if err := os.WriteFile(file, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tz := range []string{"UTC", "Europe/Berlin", "America/Los_Angeles", "Pacific/Auckland"} {
		t.Run(tz, func(t *testing.T) {
			cmd := exec.Command(node, file)
			cmd.Env = append(os.Environ(), "TZ="+tz)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("node: %v", err)
			}
			var got []string
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("output %q: %v", out, err)
			}
			for i, c := range cases {
				if got[i] != c.want {
					t.Errorf("lueckenWarnung(%q, %q) = %q, want %q", c.vorher, c.neu, got[i], c.want)
				}
			}
		})
	}
}
