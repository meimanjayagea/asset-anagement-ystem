package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeStrictJSON(t *testing.T) {
	for _, body := range []string{`{"name":"ok"} trailing`, `{"name":"ok"} {}`, `{"unknown":"x"}`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		var in struct {
			Name string `json:"name"`
		}
		if decode(w, r, &in) == nil {
			t.Fatalf("accepted invalid JSON: %s", body)
		}
	}
}
func TestOriginValidation(t *testing.T) {
	for _, s := range []string{"https://example.com/path", "ftp://example.com", "https://user:pass@example.com", "https://example.com?q=1", "garbage"} {
		if ValidOrigin(s) {
			t.Fatalf("accepted %s", s)
		}
	}
	if !ValidOrigin("https://assets.example.com") {
		t.Fatal("valid origin rejected")
	}
}

func TestCSVInjectionProtection(t *testing.T) {
	for _, v := range []string{"=SUM(A1)", " +cmd", "@cmd", "-cmd"} {
		if safeCSV(v) == v {
			t.Fatalf("unsafe CSV %q", v)
		}
	}
	if safeCSV("Normal asset") != "Normal asset" {
		t.Fatal("normal value changed")
	}
}
