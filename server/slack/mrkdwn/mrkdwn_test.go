// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mrkdwn

import "testing"

func TestRender(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"bold stars", "a **b** c", "a *b* c"},
		{"bold underscores", "a __b__ c", "a *b* c"},
		{"italic star", "a *b* c", "a _b_ c"},
		{"italic underscore unchanged", "a _b_ c", "a _b_ c"},
		{"bold then italic in one line", "**x** and *y*", "*x* and _y_"},
		{"bold italic nested", "***x***", "_*x*_"},
		{"strike", "~~x~~", "~x~"},
		{"heading", "## Title", "*Title*"},
		{"heading with trailing hashes", "# Title ##", "*Title*"},
		{"hashtag is not a heading", "#hashtag", "#hashtag"},
		{"link", "see [docs](https://x.io/a?b=1&c=2)", "see <https://x.io/a?b=1&c=2|docs>"},
		{"link text escaped once", "[a & b](https://x.io)", "<https://x.io|a &amp; b>"},
		{"bare url untouched", "go to https://x.io/a?b=1&c=2 now", "go to https://x.io/a?b=1&c=2 now"},
		{"bullet dash", "- one", "• one"},
		{"bullet star nested", "  * two", "  • two"},
		{"numbered unchanged", "1. one", "1. one"},
		{"escape amp lt gt", "a & b < c > d", "a &amp; b &lt; c &gt; d"},
		{"blockquote keeps marker", "> quoted **x**", "> quoted *x*"},
		{"inline code untouched", "run `a **b** <c>` now", "run `a **b** <c>` now"},
		{"slack mention untouched", "ask <@U123> or <#C9|general>", "ask <@U123> or <#C9|general>"},
		{"star arithmetic not italic", "2*3*4", "2*3*4"},
		{"lone stars not italic", "a * b * c", "a * b * c"},
		{"empty bold not matched", "****", "****"},
		{"snake_case not italic", "user_id and _x_", "user_id and _x_"},
		{"empty", "", ""},
		{"nul stripped", "a\x00**b**\x00", "a*b*"},
		{"trailing newline kept", "a\n", "a\n"},
		{"multiline mixed", "# H\n\n- **a**\n- b", "*H*\n\n• *a*\n• b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Render(tc.in); got != tc.want {
				t.Errorf("Render(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderFencedCodeUntouched(t *testing.T) {
	in := "before **b**\n```go\nx := a < b && **c**\n# not heading\n```\nafter **b**"
	want := "before *b*\n```go\nx := a < b && **c**\n# not heading\n```\nafter *b*"
	if got := Render(in); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestRenderUnclosedFenceCopiesToEnd(t *testing.T) {
	in := "```\n**x**\n<y>"
	if got := Render(in); got != in {
		t.Errorf("got %q, want input unchanged", got)
	}
}

func TestRenderTableBecomesCodeBlock(t *testing.T) {
	in := "intro\n| a | b |\n|---|---|\n| 1 & 2 | **x** |\noutro"
	want := "intro\n```\n| a | b |\n|---|---|\n| 1 & 2 | **x** |\n```\noutro"
	if got := Render(in); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestRenderSinglePipeLineIsNotTable(t *testing.T) {
	if got := Render("| alone |"); got != "| alone |" {
		t.Errorf("got %q", got)
	}
}
