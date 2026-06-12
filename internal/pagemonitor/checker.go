package pagemonitor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// CheckResult holds the extracted content signal and its hash.
type CheckResult struct {
	Hash    string // SHA256 hex of Content
	Content string // human-readable extracted text (stored for diff display)
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// amazonFallbackSelectors are tried in order when no user selector is given and the host looks like Amazon.
var amazonFallbackSelectors = []string{
	"#availability",
	"#price_inside_buybox",
	"#corePrice_feature_div",
	"#productTitle",
}

// skipTags are subtrees we discard when extracting visible text.
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true,
	"head": true, "nav": true, "footer": true, "iframe": true,
}

func CheckURL(ctx context.Context, rawURL, selector string) (CheckResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return CheckResult{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return CheckResult{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return CheckResult{}, err
	}

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return CheckResult{}, err
	}

	var content string
	switch {
	case selector != "":
		content = extractBySelector(doc, selector)
	case isAmazon(rawURL):
		content = extractBySelectors(doc, amazonFallbackSelectors)
	}
	if content == "" {
		content = extractVisibleText(doc)
	}
	content = normalizeWhitespace(content)

	sum := sha256.Sum256([]byte(content))
	return CheckResult{
		Hash:    fmt.Sprintf("%x", sum),
		Content: content,
	}, nil
}

func isAmazon(rawURL string) bool {
	return strings.Contains(rawURL, "amazon.")
}

// extractBySelector returns text from the first element matching the CSS selector.
func extractBySelector(doc *html.Node, sel string) string {
	compiled, err := cascadia.ParseGroup(sel)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for _, node := range cascadia.QueryAll(doc, compiled) {
		text := extractVisibleText(node)
		if text != "" {
			fmt.Fprintf(&sb, "%s\n", text)
		}
	}
	return sb.String()
}

// extractBySelectors tries each selector and concatenates non-empty results.
func extractBySelectors(doc *html.Node, selectors []string) string {
	var sb strings.Builder
	for _, sel := range selectors {
		text := extractBySelector(doc, sel)
		if text != "" {
			fmt.Fprintf(&sb, "[%s] %s\n", sel, text)
		}
	}
	return sb.String()
}

func extractVisibleText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteRune(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}
