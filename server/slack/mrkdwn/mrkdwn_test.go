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

import (
	"strings"
	"testing"
)

var renderTests = []struct{ name, in, want string }{
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
	{"bold around inline code", "**`x`**", "*`x`*"},
	{"bold around link", "**[t](https://x.io)**", "*<https://x.io|t>*"},
	{"bold around mention", "**<@U1>**", "*<@U1>*"},
	{"url followed by inline code", "see https://x.io`c`", "see https://x.io`c`"},
	{"bold url", "**https://x.io**", "*https://x.io*"},
	{"url before full stop", "see https://x.io.", "see https://x.io."},
	{"url in parens", "(https://x.io)", "(https://x.io)"},
	{"bold heading", "## **Summary**", "*Summary*"},
	{"bold inside heading", "## The **big** idea", "*The big idea*"},
	{"unbalanced stars not italic", "***b*", "***b*"},
	{"bold trailing space not matched", "**bold with trailing space **", "**bold with trailing space **"},
	{"bold leading space not matched", "** x**", "** x**"},
	{"strike spaces not matched", "~~ b ~~", "~~ b ~~"},
	{"bullet plus", "+ plus", "• plus"},
	{"seven hashes not heading", "####### seven", "####### seven"},
	{"slack special mention untouched", "tell <!here> now", "tell <!here> now"},
	{"slack link with label untouched", "<https://x.io|label>", "<https://x.io|label>"},
	{"slack link untouched", "<https://x.io>", "<https://x.io>"},
	{"heading inline code untouched", "## The `__init__` method", "*The `__init__` method*"},
	{"heading url untouched", "## See https://x.io/__init__", "*See https://x.io/__init__*"},
	{"bold url before full stop", "Docs at **https://x.io**.", "Docs at *https://x.io*."},
	{"bold url in parens", "(**https://x.io**)", "(*https://x.io*)"},
	{"struck url before full stop", "~~https://x.io~~.", "~https://x.io~."},
}

func TestRender(t *testing.T) {
	for _, tc := range renderTests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Render(tc.in); got != tc.want {
				t.Errorf("Render(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// NUL is Render's internal placeholder marker, so any NUL in the output is a
// placeholder that was never expanded.
func TestRenderNeverEmitsNUL(t *testing.T) {
	inputs := []string{
		// The block-level tests below.
		"before **b**\n```go\nx := a < b && **c**\n# not heading\n```\nafter **b**",
		"```\n**x**\n<y>",
		"intro\n| a | b |\n|---|---|\n| 1 & 2 | **x** |\noutro",
		"| alone |",
		"  ```\n**x**\n  ```",
		"|\n|",
	}
	for _, tc := range renderTests {
		inputs = append(inputs, tc.in)
	}
	for _, in := range inputs {
		if got := Render(in); strings.ContainsRune(got, 0) {
			t.Errorf("Render(%q) = %q, contains NUL", in, got)
		}
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

func TestRenderIndentedFence(t *testing.T) {
	in := "  ```\n**x**\n  ```"
	if got := Render(in); got != in {
		t.Errorf("got %q, want input unchanged", got)
	}
}

func TestRenderTwoLonePipesAreNotATable(t *testing.T) {
	in := "|\n|"
	if got := Render(in); got != in {
		t.Errorf("got %q, want input unchanged", got)
	}
}
