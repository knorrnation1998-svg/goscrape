package parser

import (
	"net"
	"net/url"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	mustParse := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	tests := []struct {
		name string
		raw  string
		base string
		want string
		ok   bool
	}{
		{"fragment and host case", "/about#team", "https://Example.com/docs/", "https://example.com/about", true},
		{"relative parent", "../x", "https://example.com/a/b/", "https://example.com/a/x", true},
		{"default https port", "https://example.com:443/p?q=1#f", "", "https://example.com/p?q=1", true},
		{"default http port and empty path", "http://example.com:80", "", "http://example.com/", true},
		{"custom port kept", "http://example.com:8080/x", "", "http://example.com:8080/x", true},
		{"credentials dropped", "https://user:pw@example.com/", "", "https://example.com/", true},
		{"ipv6 literal", "http://[::1]:8080/x", "", "http://[::1]:8080/x", true},
		{"protocol relative", "//cdn.example.com/x", "https://example.com/", "https://cdn.example.com/x", true},
		{"mailto", "mailto:a@b.c", "https://example.com/", "", false},
		{"javascript", "javascript:void(0)", "https://example.com/", "", false},
		{"tel", "tel:+123", "https://example.com/", "", false},
		{"empty", "   ", "https://example.com/", "", false},
		{"no host", "http://", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var base *url.URL
			if tc.base != "" {
				base = mustParse(tc.base)
			}
			got, ok := NormalizeURL(tc.raw, base)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NormalizeURL(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNormalizeTarget(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"example.com", "https://example.com/", false},
		{"  http://Example.com/a ", "http://example.com/a", false},
		{"localhost:3000", "https://localhost:3000/", false},
		{"", "", true},
		{"ftp://example.com", "", true},
		{"http://", "", true},
	}
	for _, tc := range tests {
		got, err := NormalizeTarget(tc.raw)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("NormalizeTarget(%q) = (%q, %v), want (%q, err=%v)", tc.raw, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestIsDisallowedIP(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "172.16.0.1", "169.254.169.254", "::1", "100.64.0.1", "0.0.0.0", "fe80::1"}
	allowed := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"}
	for _, s := range blocked {
		if !isDisallowedIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range allowed {
		if isDisallowedIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestCleanAndTruncate(t *testing.T) {
	if got := clean("  a \n b\t c  "); got != "a b c" {
		t.Errorf("clean = %q", got)
	}
	if got := truncate("héllo wörld", 5); got != "héllo…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate must not touch short strings, got %q", got)
	}
}

func TestIsHTML(t *testing.T) {
	tests := []struct {
		contentType string
		body        string
		want        bool
	}{
		{"text/html; charset=utf-8", "", true},
		{"application/xhtml+xml", "", true},
		{"application/json", "{}", false},
		{"image/png", "", false},
		{"", "<!DOCTYPE html><html></html>", true},
		{"", "plain text", false},
	}
	for _, tc := range tests {
		if got := isHTML(tc.contentType, []byte(tc.body)); got != tc.want {
			t.Errorf("isHTML(%q, %q) = %v, want %v", tc.contentType, tc.body, got, tc.want)
		}
	}
}
