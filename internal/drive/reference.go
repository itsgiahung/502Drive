package drive

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

type Reference struct {
	ID  string `json:"id"`
	Key string `json:"key,omitempty"`
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,256}$`)
var validKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

func Parse(input string) (Reference, error) {
	s := strings.TrimSpace(input)
	if validID.MatchString(s) {
		return Reference{ID: s}, nil
	}
	bad := errors.New("expected one Google Drive ID or HTTPS Drive/Docs link")
	if strings.HasPrefix(s, "drive.google.com/") || strings.HasPrefix(s, "docs.google.com/") {
		s = "https://" + s
	}
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (u.Hostname() != "drive.google.com" && u.Hostname() != "docs.google.com") {
		return Reference{}, bad
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return Reference{}, bad
	}
	r := Reference{}
	for _, k := range []string{"id", "fileId", "folderId"} {
		if len(q[k]) > 1 {
			return r, bad
		}
		if v := q.Get(k); v != "" {
			if r.ID != "" {
				return Reference{}, bad
			}
			r.ID = v
		}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "d" || parts[i] == "folders" {
			if r.ID != "" {
				return Reference{}, bad
			}
			r.ID = parts[i+1]
		}
	}
	for _, k := range []string{"resourcekey", "resourceKey"} {
		if len(q[k]) > 1 {
			return Reference{}, bad
		}
		if v := q.Get(k); v != "" {
			if r.Key != "" {
				return Reference{}, bad
			}
			r.Key = v
		}
	}
	if !validID.MatchString(r.ID) || (r.Key != "" && !validKey.MatchString(r.Key)) {
		return Reference{}, bad
	}
	return r, nil
}
