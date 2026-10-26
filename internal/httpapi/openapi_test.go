package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// A small JSON-Schema subset validator, enough to check responses against
// docs/contracts/openapi.json (type, enum, const, required, properties,
// additionalProperties, items, anyOf, oneOf, bounds, date-time).
type spec struct{ doc map[string]any }

func loadSpec(t *testing.T) *spec {
	t.Helper()
	b, err := os.ReadFile("../../docs/contracts/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err = json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return &spec{doc: d}
}

func (s *spec) ref(r string) any {
	var cur any = s.doc
	for _, p := range strings.Split(strings.TrimPrefix(r, "#/"), "/") {
		cur = cur.(map[string]any)[p]
	}
	return cur
}

func typeOf(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if x == float64(int64(x)) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "?"
}

func (s *spec) validate(schema any, v any, path string) []string {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	if r, ok := m["$ref"].(string); ok {
		return s.validate(s.ref(r), v, path)
	}
	var errs []string
	if alts, ok := m["anyOf"].([]any); ok {
		for _, a := range alts {
			if len(s.validate(a, v, path)) == 0 {
				return nil
			}
		}
		return []string{path + ": matches no anyOf branch"}
	}
	if alts, ok := m["oneOf"].([]any); ok {
		n := 0
		for _, a := range alts {
			if len(s.validate(a, v, path)) == 0 {
				n++
			}
		}
		if n != 1 {
			return []string{fmt.Sprintf("%s: matches %d oneOf branches", path, n)}
		}
		return nil
	}
	if c, ok := m["const"]; ok && fmt.Sprint(c) != fmt.Sprint(v) {
		errs = append(errs, fmt.Sprintf("%s: want const %v got %v", path, c, v))
	}
	if e, ok := m["enum"].([]any); ok {
		found := false
		for _, x := range e {
			if fmt.Sprint(x) == fmt.Sprint(v) {
				found = true
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("%s: %v not in enum %v", path, v, e))
		}
	}
	if t, ok := m["type"]; ok {
		want := []string{}
		switch x := t.(type) {
		case string:
			want = append(want, x)
		case []any:
			for _, y := range x {
				want = append(want, y.(string))
			}
		}
		got := typeOf(v)
		match := false
		for _, w := range want {
			if w == got || (w == "number" && got == "integer") {
				match = true
			}
		}
		if !match {
			return append(errs, fmt.Sprintf("%s: type %s, want %v", path, got, want))
		}
	}
	switch x := v.(type) {
	case string:
		if n, ok := m["minLength"].(float64); ok && float64(len([]rune(x))) < n {
			errs = append(errs, path+": too short")
		}
		if n, ok := m["maxLength"].(float64); ok && float64(len([]rune(x))) > n {
			errs = append(errs, path+": too long")
		}
		if m["format"] == "date-time" {
			if _, err := time.Parse(time.RFC3339Nano, x); err != nil {
				errs = append(errs, path+": bad date-time")
			}
		}
	case float64:
		if n, ok := m["minimum"].(float64); ok && x < n {
			errs = append(errs, path+": below minimum")
		}
		if n, ok := m["maximum"].(float64); ok && x > n {
			errs = append(errs, path+": above maximum")
		}
	case []any:
		if it, ok := m["items"]; ok {
			for i, e := range x {
				errs = append(errs, s.validate(it, e, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	case map[string]any:
		props, _ := m["properties"].(map[string]any)
		if req, ok := m["required"].([]any); ok {
			for _, r := range req {
				if _, ok := x[r.(string)]; !ok {
					errs = append(errs, fmt.Sprintf("%s: missing required %q", path, r))
				}
			}
		}
		for k, val := range x {
			if ps, ok := props[k]; ok {
				errs = append(errs, s.validate(ps, val, path+"."+k)...)
			} else if ap, ok := m["additionalProperties"].(bool); ok && !ap {
				errs = append(errs, fmt.Sprintf("%s: unexpected property %q", path, k))
			}
		}
	}
	return errs
}

// check validates a response body against the documented response for
// (method, path template, status).
func (s *spec) check(method, tmpl string, status int, body []byte) []string {
	paths := s.doc["paths"].(map[string]any)
	op, ok := paths[tmpl].(map[string]any)[strings.ToLower(method)].(map[string]any)
	if !ok {
		return []string{"operation not in spec: " + method + " " + tmpl}
	}
	responses := op["responses"].(map[string]any)
	resp, ok := responses[fmt.Sprint(status)]
	if !ok {
		resp, ok = responses["default"]
		if !ok {
			return []string{fmt.Sprintf("status %d not documented for %s %s", status, method, tmpl)}
		}
	}
	rm := resp.(map[string]any)
	if r, ok := rm["$ref"].(string); ok {
		rm = s.ref(r).(map[string]any)
	}
	content, ok := rm["content"].(map[string]any)
	if !ok {
		if len(body) != 0 {
			return []string{"unexpected body for bodiless response"}
		}
		return nil
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return []string{"body is not JSON: " + err.Error()}
	}
	return s.validate(content["application/json"].(map[string]any)["schema"], v, "$")
}
