// Package filter implements selective sync (S5): which entries of a source
// tree are copied. It is pure: the engine calls Decide for each listed entry
// during discovery, so filtered entries are never copied and never held in
// memory as a whole tree.
package filter

import (
	"errors"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
	"time"
)

// Spec is the stored filter of a plan, job or collection. The zero value keeps
// everything. Rules are strings so the JSON stays readable and stable:
//
//	glob:        "*.tmp", "docs/**", "**/cache/*"  (a pattern without "/" matches the name)
//	extension:   "ext:.pdf"  (or "ext:pdf")
//	mime class:  "class:image|video|audio|doc|archive|native|other"
//
// Include: when non-empty, an entry must match at least one include rule.
// Exclude: any match drops the entry, and exclude always wins over include.
// Folders are only pruned by exclude rules; a folder that ends up with nothing
// kept is not created unless KeepEmptyFolders is set (see the engine).
type Spec struct {
	Include          []string `json:"include,omitempty"`
	Exclude          []string `json:"exclude,omitempty"`
	MinSize          int64    `json:"min_size,omitempty"`
	MaxSize          int64    `json:"max_size,omitempty"`
	ModifiedAfter    string   `json:"modified_after,omitempty"` // RFC 3339
	KeepEmptyFolders bool     `json:"keep_empty_folders,omitempty"`
}

// Active reports whether any rule would drop something. A spec that only sets
// KeepEmptyFolders changes nothing and is not active.
func (s Spec) Active() bool {
	return len(s.Include) > 0 || len(s.Exclude) > 0 || s.MinSize > 0 || s.MaxSize > 0 || s.ModifiedAfter != ""
}

var classes = map[string]bool{"image": true, "video": true, "audio": true, "doc": true, "archive": true, "native": true, "other": true}

// Validate reports the first problem with a spec, without compiling it.
func (s Spec) Validate() error {
	_, err := s.Compile()
	return err
}

// Matcher is a compiled Spec.
type Matcher struct {
	include, exclude []rule
	minSize, maxSize int64
	modifiedAfter    time.Time
	hasModified      bool
	spec             Spec
}

type rule struct {
	kind  string // glob, ext or class
	value string
}

// Compile validates the spec and prepares it for matching.
func (s Spec) Compile() (*Matcher, error) {
	m := &Matcher{spec: s, minSize: s.MinSize, maxSize: s.MaxSize}
	var err error
	if m.include, err = compileRules(s.Include, "include"); err != nil {
		return nil, err
	}
	if m.exclude, err = compileRules(s.Exclude, "exclude"); err != nil {
		return nil, err
	}
	if s.MinSize < 0 || s.MaxSize < 0 {
		return nil, errors.New("size bounds cannot be negative")
	}
	if s.MaxSize > 0 && s.MinSize > s.MaxSize {
		return nil, errors.New("min_size is larger than max_size")
	}
	if s.ModifiedAfter != "" {
		t, err := time.Parse(time.RFC3339, s.ModifiedAfter)
		if err != nil {
			return nil, errors.New("modified_after must be an RFC 3339 time")
		}
		m.modifiedAfter, m.hasModified = t, true
	}
	return m, nil
}

func compileRules(in []string, field string) ([]rule, error) {
	var out []rule
	for _, raw := range in {
		r := strings.TrimSpace(raw)
		switch {
		case r == "":
			return nil, fmt.Errorf("%s has an empty rule", field)
		case strings.HasPrefix(r, "ext:"):
			ext := strings.ToLower(strings.TrimPrefix(r[len("ext:"):], "."))
			if ext == "" || strings.ContainsAny(ext, "/*?[") {
				return nil, fmt.Errorf("%s: invalid extension %q", field, raw)
			}
			out = append(out, rule{kind: "ext", value: "." + ext})
		case strings.HasPrefix(r, "class:"):
			c := r[len("class:"):]
			if !classes[c] {
				return nil, fmt.Errorf("%s: unknown class %q (image, video, audio, doc, archive, native, other)", field, c)
			}
			out = append(out, rule{kind: "class", value: c})
		default:
			if _, err := path.Match(r, ""); err != nil {
				return nil, fmt.Errorf("%s: bad pattern %q", field, raw)
			}
			out = append(out, rule{kind: "glob", value: r})
		}
	}
	return out, nil
}

// Entry is what a listing tells us about one object.
type Entry struct {
	Name     string    // name in Drive
	Rel      string    // path relative to the copied root, "/"-separated
	IsFolder bool      // folders are pruned only by exclude globs
	MimeType string    // decides the mime class
	Size     *int64    // nil for Google native files and folders
	Modified time.Time // zero when unknown (the rule then passes)
}

// Decide reports whether the entry is kept.
func (m *Matcher) Decide(e Entry) bool {
	for _, r := range m.exclude {
		if e.IsFolder && r.kind != "glob" {
			continue
		}
		if matches(r, e) {
			return false
		}
	}
	if e.IsFolder {
		return true
	}
	if len(m.include) > 0 {
		hit := false
		for _, r := range m.include {
			if matches(r, e) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if e.Size != nil {
		if m.minSize > 0 && *e.Size < m.minSize {
			return false
		}
		if m.maxSize > 0 && *e.Size > m.maxSize {
			return false
		}
	}
	if m.hasModified && !e.Modified.IsZero() && e.Modified.Before(m.modifiedAfter) {
		return false
	}
	return true
}

func matches(r rule, e Entry) bool {
	switch r.kind {
	case "ext":
		return strings.HasSuffix(strings.ToLower(e.Name), r.value)
	case "class":
		return MimeClass(e.MimeType, e.IsFolder) == r.value
	}
	if strings.Contains(r.value, "/") {
		return globPath(r.value, e.Rel)
	}
	ok, _ := path.Match(r.value, e.Name)
	return ok
}

// globPath matches a slash pattern against a relative path. "**" as a whole
// segment matches any number of segments (including none).
func globPath(pattern, rel string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(p, s []string) bool {
	if len(p) == 0 {
		return len(s) == 0
	}
	if p[0] == "**" {
		for i := 0; i <= len(s); i++ {
			if matchSegments(p[1:], s[i:]) {
				return true
			}
		}
		return false
	}
	if len(s) == 0 {
		return false
	}
	ok, _ := path.Match(p[0], s[0])
	return ok && matchSegments(p[1:], s[1:])
}

// MimeClass maps a Drive mime type to the class names used in rules.
func MimeClass(mime string, folder bool) string {
	switch {
	case folder:
		return "other"
	case strings.HasPrefix(mime, "application/vnd.google-apps."):
		return "native"
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.HasPrefix(mime, "text/"), mime == "application/pdf", mime == "application/rtf",
		strings.Contains(mime, "msword"), strings.Contains(mime, "ms-excel"), strings.Contains(mime, "ms-powerpoint"),
		strings.Contains(mime, "officedocument"), strings.Contains(mime, "opendocument"):
		return "doc"
	}
	switch mime {
	case "application/zip", "application/x-tar", "application/gzip", "application/x-gzip", "application/x-7z-compressed",
		"application/x-rar-compressed", "application/vnd.rar", "application/x-bzip2", "application/x-xz", "application/zstd":
		return "archive"
	}
	return "other"
}

// ParseSize reads a byte count with an optional KB, MB, GB or TB suffix
// (powers of 1024; a bare number is bytes).
func ParseSize(s string) (int64, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		m      int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(t, u.suffix) {
			mult = u.m
			t = strings.TrimSpace(strings.TrimSuffix(t, u.suffix))
			break
		}
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil || n < 0 || n > math.MaxInt64/mult {
		return 0, fmt.Errorf("invalid size %q (use bytes or a KB, MB, GB, TB suffix)", s)
	}
	return n * mult, nil
}
