package web

import (
	"os"
	"testing"
)

// TestMain keeps the update check off the internet: every dashboard request
// with a stale cache starts a refresh in the background, and a real answer
// would overwrite what setReleaseCache just wrote. A failing refresh leaves
// the cache alone, so both URLs point at an address that fails at once.
func TestMain(m *testing.M) {
	releaseAPIURL, nightlyConfigURL = "http://127.0.0.1:0", "http://127.0.0.1:0"
	os.Exit(m.Run())
}
