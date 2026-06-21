package pagemonitor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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

// blockTags are visible element boundaries worth preserving in extracted text.
var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"br": true, "caption": true, "dd": true, "details": true, "div": true,
	"dl": true, "dt": true, "fieldset": true, "figcaption": true, "figure": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "header": true, "hr": true, "li": true, "main": true,
	"ol": true, "p": true, "pre": true, "section": true, "summary": true,
	"table": true, "tbody": true, "td": true, "tfoot": true, "th": true,
	"thead": true, "tr": true, "ul": true,
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Println("Error closing body:", err)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return CheckResult{}, err
	}

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return CheckResult{}, err
	}

	selectors := make([]string, 0)
	for s := range strings.SplitSeq(selector, ",") {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			selectors = append(selectors, trimmed)
		}
	}
	if len(selectors) == 0 && isAmazon(rawURL) {
		selectors = amazonFallbackSelectors
	}

	var content string
	if len(selectors) > 0 {
		content = extractBySelectors(doc, selectors)
	}
	if content == "" {
		content = extractVisibleText(doc)
	}
	content = normalizeExtractedText(content)

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
			fmt.Fprintf(&sb, "%s\n\n", text)
		}
	}
	return strings.TrimSpace(sb.String())
}

// extractBySelectors tries each selector and concatenates non-empty results.
func extractBySelectors(doc *html.Node, selectors []string) string {
	var sb strings.Builder
	for _, sel := range selectors {
		text := extractBySelector(doc, sel)
		if text != "" {
			fmt.Fprintf(&sb, "[%s]\n%s\n\n", sel, text)
		}
	}
	return strings.TrimSpace(sb.String())
}

func extractVisibleText(n *html.Node) string {
	var lines []string
	var line strings.Builder

	flushLine := func() {
		text := strings.Join(strings.Fields(line.String()), " ")
		line.Reset()
		if text == "" {
			return
		}
		lines = append(lines, text)
	}
	writeText := func(text string) {
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			return
		}
		if line.Len() > 0 {
			line.WriteRune(' ')
		}
		line.WriteString(text)
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			flushLine()
			return
		}
		if n.Type == html.ElementNode && blockTags[n.Data] && line.Len() > 0 {
			flushLine()
		}
		if n.Type == html.TextNode {
			writeText(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			flushLine()
		}
	}
	walk(n)
	flushLine()
	return strings.Join(lines, "\n")
}

func normalizeExtractedText(s string) string {
	lines := strings.Split(s, "\n")
	normalized := make([]string, 0, len(lines))
	previousBlank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(normalized) > 0 && !previousBlank {
				normalized = append(normalized, "")
				previousBlank = true
			}
			continue
		}
		normalized = append(normalized, line)
		previousBlank = false
	}
	for len(normalized) > 0 && normalized[len(normalized)-1] == "" {
		normalized = normalized[:len(normalized)-1]
	}
	return strings.Join(normalized, "\n")
}
