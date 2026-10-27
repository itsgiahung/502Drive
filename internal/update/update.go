// Package update checks whether a newer 502Drive release exists and, when the
// user runs `502drive update --install`, installs it after verifying its
// checksum. Nothing happens on its own: the tray only checks, and only when the
// user opted in.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Repo is the project's GitHub repository.
const Repo = "itsgiahung/502Drive"

// Release is the latest published release.
type Release struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

// ErrNoRelease means the repository has no published release (or is private).
var ErrNoRelease = errors.New("no published release found")

// Latest reads releases/latest. base is the API root ("" = GitHub).
func Latest(ctx context.Context, hc *http.Client, base string) (Release, error) {
	if base == "" {
		base = "https://api.github.com"
	}
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "502drive-update-check")
	res, err := hc.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("cannot reach the release server: %w", err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Release{}, ErrNoRelease
	default:
		return Release{}, fmt.Errorf("the release server answered HTTP %d", res.StatusCode)
	}
	var r Release
	if err = json.NewDecoder(http.MaxBytesReader(nil, res.Body, 1<<20)).Decode(&r); err != nil || r.Tag == "" {
		return Release{}, errors.New("the release metadata is unreadable")
	}
	return r, nil
}

// Newer reports whether tag is a strictly newer version than current. A
// development build ("dev") or anything unparsable is never "older".
func Newer(current, tag string) bool {
	c, ok1 := parse(current)
	t, ok2 := parse(tag)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
