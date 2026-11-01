package bot

import "testing"

func TestRewriteTweetLinks(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		changed bool
	}{
		{
			in:      "https://x.com/thdxr/status/2116735229792522518?a=7xJcZuXh2-RPvE_2rXY&s=19",
			want:    "https://fixupx.com/thdxr/status/2116735229792522518/en",
			changed: true,
		},
		{
			in:      "look at this https://twitter.com/a_b/status/1/photo/1?s=20 lol",
			want:    "look at this https://fixupx.com/a_b/status/1/en lol",
			changed: true,
		},
		{in: "<https://x.com/a/status/1?s=1>", want: "<https://x.com/a/status/1?s=1>"},
		{in: "https://example.com/x.com/a/status/1", want: "https://example.com/x.com/a/status/1"},
		{in: "no links", want: "no links"},
	}
	for _, tt := range tests {
		got, changed := rewriteTweetLinks(tt.in)
		if got != tt.want || changed != tt.changed {
			t.Errorf("rewriteTweetLinks(%q) = %q, %v; want %q, %v", tt.in, got, changed, tt.want, tt.changed)
		}
	}
}
