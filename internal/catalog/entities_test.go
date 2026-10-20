package catalog

import "testing"

func TestCleanTextDecodesOnlyCommonEntities(t *testing.T) {
	cases := map[string]string{
		"Don&#39;t Stop &amp; Go &quot;Live&quot;": `Don't Stop & Go "Live"`,
		"&lt;b&gt; &apos;x&apos;":                  "<b> 'x'",
		"&#x4E2D;&#x6587; &#233;":                  "中文 é",
		"&amp;amp;":                                "&amp;",
		"Tom&regina":                               "Tom&regina",
		"&reg; &nbsp; &AMP;":                       "&reg; &nbsp; &AMP;",
		"&#39 and &ampy":                           "&#39 and &ampy",
		"&amp no semicolon":                        "&amp no semicolon",
		"&#X41; &#99999999999; &#0;":               "&#X41; &#99999999999; &#0;",
		"  padded  ":                               "padded",
	}

	for raw, want := range cases {
		if got := cleanText(raw); got != want {
			t.Errorf("cleanText(%q) = %q, want %q", raw, got, want)
		}
	}
}
