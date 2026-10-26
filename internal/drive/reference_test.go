package drive

import "testing"

func TestReferencesRejectForeignHostsAndHeaderInjection(t *testing.T) {
	for _, v := range []string{"https://evil.test/file/d/abcdef12345", "https://drive.google.com.evil.test/open?id=abcdef12345", "https://user:pass@drive.google.com/open?id=abcdef12345", "https://drive.google.com/open?id=abcdef12345&resourcekey=bad%0d%0aHeader", "https://drive.google.com/open?id=abcdef12345&id=other123456", "abc", "http://drive.google.com/file/d/abcdef12345"} {
		if _, e := Parse(v); e == nil {
			t.Errorf("accepted %q", v)
		}
	}
	for _, v := range []string{"abcdef12345", "https://drive.google.com/drive/folders/abcdef12345?resourcekey=key_123", "https://docs.google.com/document/d/abcdef12345/edit"} {
		r, e := Parse(v)
		if e != nil || r.ID != "abcdef12345" {
			t.Errorf("parse %q: %+v %v", v, r, e)
		}
	}
}
