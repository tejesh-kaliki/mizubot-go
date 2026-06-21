package pagemonitor

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseHTML(t *testing.T, body string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return doc
}

func TestExtractBySelectorsPreservesListStructure(t *testing.T) {
	doc := parseHTML(t, `
		<html>
			<body>
				<div id="availability"> In stock </div>
				<div class="offers">
					<ul>
						<li><span>Bank Offer</span><span>Upto Rs. 1,177.42 discount</span><span>12 offers</span></li>
						<li><span>No Cost EMI</span><span>Upto Rs. 876.55 savings</span><span>3 offers</span></li>
					</ul>
				</div>
			</body>
		</html>`)

	got := extractBySelectors(doc, []string{"#availability", ".offers"})
	want := strings.TrimSpace(`
[#availability]
In stock

[.offers]
Bank Offer Upto Rs. 1,177.42 discount 12 offers
No Cost EMI Upto Rs. 876.55 savings 3 offers`)

	if got != want {
		t.Fatalf("extractBySelectors mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestExtractVisibleTextKeepsInlineTextTogether(t *testing.T) {
	doc := parseHTML(t, `
		<div>
			<span>Price</span>
			<span>Rs.</span><span>15,699</span>
			<p>Delivery <strong>Tomorrow</strong></p>
		</div>`)

	got := extractVisibleText(doc)
	want := strings.TrimSpace(`
Price Rs. 15,699
Delivery Tomorrow`)

	if got != want {
		t.Fatalf("extractVisibleText mismatch:\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}
