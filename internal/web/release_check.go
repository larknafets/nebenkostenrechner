package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	releaseAPIURL = "https://api.github.com/repos/larknafets/nebenkostenrechner/releases/latest"
	// nightlyConfigURL is the nightly add-on's config.yaml: nightly.yml writes
	// the version of every nightly build into it, so its `version:` line is the
	// latest nightly (the images on ghcr.io need a login to list their tags).
	nightlyConfigURL   = "https://raw.githubusercontent.com/larknafets/ha-addons/main/nebenkostenrechner-nightly/config.yaml"
	repoURL            = "https://github.com/larknafets/nebenkostenrechner"
	releaseCacheTTL    = 6 * time.Hour
	releaseHTTPTimeout = 3 * time.Second
)

type releaseCacheEntry struct {
	latest    string
	checkedAt time.Time
}

var (
	releaseCacheMu sync.Mutex
	releaseCache   releaseCacheEntry
)

// checkForUpdate reports the latest known version of currentVersion's channel
// (a GitHub release tag, or the latest nightly for a nightly build) and
// whether it's newer than currentVersion. Never blocks the caller: a stale/empty
// cache triggers a background refresh and this returns immediately with
// whatever's cached so far (possibly ""). Silent on any error (offline
// HA-Add-on, GitHub unreachable, unparsable version) - update-checking is
// a nice-to-have, never a reason to slow or break the Dashboard.
func checkForUpdate(currentVersion string) (latest string, available bool) {
	releaseCacheMu.Lock()
	entry := releaseCache
	stale := time.Since(entry.checkedAt) > releaseCacheTTL
	releaseCacheMu.Unlock()

	if stale {
		go refreshReleaseCache(currentVersion)
	}

	if entry.latest == "" {
		return "", false
	}
	return entry.latest, isNewerVersion(entry.latest, currentVersion)
}

// handleUpdateCheck serves checkForUpdate over JSON so the Dashboard's
// client-side JS can re-trigger a cache refresh on tab interaction, not
// just on the next full page load (Ticket: interaction-triggered update
// check).
func handleUpdateCheck(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		latest, available := checkForUpdate(version)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Available bool   `json:"available"`
			Latest    string `json:"latest"`
			URL       string `json:"url"`
		}{Available: available, Latest: latest, URL: updateURL(latest)})
	}
}

// PrimeUpdateCheck fills the release cache once in the background, called at
// app start so the first Dashboard already knows about a new release instead
// of only the one after it. Silent on errors like every update check.
func PrimeUpdateCheck(currentVersion string) { go refreshReleaseCache(currentVersion) }

// refreshReleaseCache asks the channel currentVersion belongs to: a nightly
// build compares against the latest nightly, everything else against the
// latest GitHub release.
func refreshReleaseCache(currentVersion string) {
	client := &http.Client{Timeout: releaseHTTPTimeout}
	var latest string
	var err error
	if _, _, ok := parseNightly(currentVersion); ok {
		latest, err = fetchLatestNightlyFrom(nightlyConfigURL, client)
	} else {
		latest, err = fetchLatestReleaseFrom(releaseAPIURL, client)
	}
	if err != nil {
		return
	}
	releaseCacheMu.Lock()
	releaseCache = releaseCacheEntry{latest: latest, checkedAt: time.Now()}
	releaseCacheMu.Unlock()
}

// fetchLatestReleaseFrom reads a GitHub "releases/latest"-shaped response
// (just the fields this needs) from url via client.
func fetchLatestReleaseFrom(url string, client *http.Client) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("releases/latest: status %d", resp.StatusCode)
	}

	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.TagName, nil
}

// nightlyRe matches the version of a nightly build, "nightly.YYYYMMDD-<sha7>"
// (.github/workflows/nightly.yml).
var nightlyRe = regexp.MustCompile(`^nightly\.(\d{8})-([0-9a-f]{7})$`)

// parseNightly splits a nightly version into its build date and commit.
// ok=false for anything else.
func parseNightly(v string) (date, sha string, ok bool) {
	m := nightlyRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// fetchLatestNightlyFrom reads the `version:` line of the nightly add-on's
// config.yaml from url. An error unless it is a nightly version.
func fetchLatestNightlyFrom(url string, client *http.Client) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nightly config: status %d", resp.StatusCode)
	}

	sc := bufio.NewScanner(io.LimitReader(resp.Body, 64<<10))
	for sc.Scan() {
		rest, found := strings.CutPrefix(sc.Text(), "version:")
		if !found {
			continue
		}
		v := strings.Trim(strings.TrimSpace(rest), `"'`)
		if _, _, ok := parseNightly(v); !ok {
			return "", fmt.Errorf("nightly config: %q is no nightly version", v)
		}
		return v, nil
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("nightly config: no version line")
}

// updateURL is where the update hint leads: the release page, or for a
// nightly the commit it was built from.
func updateURL(latest string) string {
	if _, sha, ok := parseNightly(latest); ok {
		return repoURL + "/commit/" + sha
	}
	return repoURL + "/releases/tag/" + latest
}

// parseSemver parses a "vX.Y.Z" tag (leading "v" optional) into its 3
// numeric components. ok=false for anything else - a date-based or
// otherwise non-semver tag never triggers a false "update available".
func parseSemver(v string) (major, minor, patch int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

// isNewerVersion reports whether latest is a strictly greater semver than
// current, or, for two nightly versions, a later build date. A second build
// on the same day is not told apart (the sha has no order). false whenever
// either fails to parse or the channels differ - dev builds ("" or "dev"),
// non-semver tags, or a fetch that returned garbage never claim an update
// exists.
func isNewerVersion(latest, current string) bool {
	if lDate, _, ok := parseNightly(latest); ok {
		cDate, _, ok := parseNightly(current)
		return ok && lDate > cDate
	}
	lMaj, lMin, lPatch, ok1 := parseSemver(latest)
	cMaj, cMin, cPatch, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return false
	}
	if lMaj != cMaj {
		return lMaj > cMaj
	}
	if lMin != cMin {
		return lMin > cMin
	}
	return lPatch > cPatch
}
