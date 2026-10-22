// Package parser fetches HTML documents over HTTP and extracts structured
// data (title, meta tags, headings, links) from them using goquery.
package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

const (
	// MaxLinksPerPage caps how many distinct links are extracted from one page.
	MaxLinksPerPage = 2000

	maxHeadings     = 200
	maxMetaEntries  = 100
	maxTextLen      = 300
	maxMetaValueLen = 1000
)

// Heading is a single h1-h6 element.
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// Page is the result of fetching and parsing one URL.
type Page struct {
	URL         string // final URL after redirects
	StatusCode  int
	ContentType string
	Title       string
	Meta        map[string]string
	Headings    []Heading
	Links       []string // absolute, normalised, http(s) only, de-duplicated
	Bytes       int64    // decoded body bytes read
	Duration    time.Duration
}

// Options configures a Fetcher.
type Options struct {
	UserAgent    string
	Timeout      time.Duration // whole-request timeout, including body read
	MaxBodyBytes int64         // bodies larger than this are truncated
	MaxRedirects int
	// AllowPrivate permits connections to loopback, private and link-local
	// addresses. It is off by default so the server cannot be used to probe
	// internal networks (SSRF).
	AllowPrivate bool
}

// Fetcher downloads pages. It is safe for concurrent use.
type Fetcher struct {
	client *http.Client
	opts   Options
}

// NewFetcher builds a Fetcher, filling in defaults for unset options.
//
// The transport deliberately does not honour proxy environment variables:
// the address guard below must see the real destination IP.
func NewFetcher(opts Options) *Fetcher {
	if opts.UserAgent == "" {
		opts.UserAgent = "GoScrape/1.0 (+https://github.com/goscrape)"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 5 << 20
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = 5
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if !opts.AllowPrivate {
		dialer.Control = blockPrivateAddresses
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	maxRedirects := opts.MaxRedirects
	client := &http.Client{
		Transport: transport,
		Timeout:   opts.Timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
	return &Fetcher{client: client, opts: opts}
}

// Fetch downloads rawURL and, when the response is HTML, parses it. HTTP error
// statuses are not errors: they are returned in Page.StatusCode. An error is
// returned only when no usable response was obtained.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.opts.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "en")

	start := time.Now()
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.opts.MaxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	page := &Page{
		URL:         resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Meta:        map[string]string{},
		Headings:    []Heading{},
		Links:       []string{},
		Bytes:       int64(len(body)),
		Duration:    time.Since(start),
	}
	if len(body) == 0 || !isHTML(page.ContentType, body) {
		return page, nil
	}
	if err := page.extract(body, resp.Request.URL); err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	return page, nil
}

func isHTML(contentType string, body []byte) bool {
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.Contains(strings.ToLower(contentType), "html")
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

// extract fills the title, meta, heading and link fields from an HTML body.
func (p *Page) extract(body []byte, base *url.URL) error {
	var reader io.Reader = bytes.NewReader(body)
	if converted, err := charset.NewReader(bytes.NewReader(body), p.ContentType); err == nil {
		reader = converted
	}
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return err
	}

	p.Title = truncate(clean(doc.Find("title").First().Text()), maxTextLen)

	if lang, ok := doc.Find("html").First().Attr("lang"); ok && strings.TrimSpace(lang) != "" {
		p.Meta["lang"] = strings.TrimSpace(lang)
	}
	if href, ok := doc.Find(`link[rel="canonical"]`).First().Attr("href"); ok {
		if abs, ok := NormalizeURL(href, base); ok {
			p.Meta["canonical"] = abs
		}
	}
	doc.Find("meta").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if len(p.Meta) >= maxMetaEntries {
			return false
		}
		if cs, ok := s.Attr("charset"); ok && strings.TrimSpace(cs) != "" {
			p.Meta["charset"] = strings.TrimSpace(cs)
			return true
		}
		content, ok := s.Attr("content")
		content = truncate(strings.TrimSpace(content), maxMetaValueLen)
		if !ok || content == "" {
			return true
		}
		for _, attr := range []string{"name", "property", "http-equiv", "itemprop"} {
			key, has := s.Attr(attr)
			key = strings.ToLower(strings.TrimSpace(key))
			if !has || key == "" {
				continue
			}
			if _, exists := p.Meta[key]; !exists {
				p.Meta[key] = content
			}
			break
		}
		return true
	})

	doc.Find("h1, h2, h3, h4, h5, h6").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if len(p.Headings) >= maxHeadings {
			return false
		}
		name := goquery.NodeName(s)
		text := clean(s.Text())
		if len(name) != 2 || text == "" {
			return true
		}
		p.Headings = append(p.Headings, Heading{
			Level: int(name[1] - '0'),
			Text:  truncate(text, maxTextLen),
		})
		return true
	})

	linkBase := base
	if href, ok := doc.Find("base[href]").First().Attr("href"); ok {
		if resolved, err := base.Parse(strings.TrimSpace(href)); err == nil {
			linkBase = resolved
		}
	}
	seen := make(map[string]struct{})
	doc.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if len(p.Links) >= MaxLinksPerPage {
			return false
		}
		href, _ := s.Attr("href")
		abs, ok := NormalizeURL(href, linkBase)
		if !ok {
			return true
		}
		if _, dup := seen[abs]; dup {
			return true
		}
		seen[abs] = struct{}{}
		p.Links = append(p.Links, abs)
		return true
	})
	return nil
}

// NormalizeURL resolves raw against base (which may be nil) and returns a
// canonical absolute http(s) URL: lower-case host, no fragment, no credentials,
// default ports removed and an explicit "/" path. The second result is false
// for anything that is not a crawlable http(s) URL (mailto:, javascript:, ...).
func NormalizeURL(raw string, base *url.URL) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	var (
		u   *url.URL
		err error
	)
	if base != nil {
		u, err = base.Parse(raw)
	} else {
		u, err = url.Parse(raw)
	}
	if err != nil {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	if strings.Contains(host, ":") { // IPv6 literal
		host = "[" + host + "]"
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	u.Host = host
	u.User = nil
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), true
}

// NormalizeTarget validates user input for a crawl start URL. A missing scheme
// defaults to https.
func NormalizeTarget(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	normalized, ok := NormalizeURL(raw, nil)
	if !ok {
		return "", fmt.Errorf("%q is not a valid http(s) URL", raw)
	}
	return normalized, nil
}

// clean collapses all runs of whitespace into single spaces.
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncate shortens s to at most n runes, appending an ellipsis if it cut.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

var carrierGradeNAT = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func isDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() ||
		carrierGradeNAT.Contains(ip)
}

// blockPrivateAddresses is a net.Dialer Control hook. It runs after DNS
// resolution, so it also defeats hostnames that resolve to internal IPs.
func blockPrivateAddresses(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("refusing to connect to non-IP address %q", host)
	}
	if isDisallowedIP(ip) {
		return fmt.Errorf("refusing to connect to private or loopback address %s (start the server with -allow-private to permit this)", ip)
	}
	return nil
}
