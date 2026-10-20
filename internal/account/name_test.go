package account

import (
	"errors"
	"strings"
	"testing"
)

const (
	england  = "\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F"
	scotland = "\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F"
	wales    = "\U0001F3F4\U000E0067\U000E0062\U000E0077\U000E006C\U000E0073\U000E007F"
)

func TestParseDisplayName(t *testing.T) {
	accepted := map[string]DisplayName{
		"  Ann  ":               "Ann",
		"แอน 🎧":                 "แอน 🎧",
		strings.Repeat("ก", 40): DisplayName(strings.Repeat("ก", 40)),
		"Ann (DJ) #1!":          "Ann (DJ) #1!",
		"👩\u200d🎤":              "👩\u200d🎤",
		"❤\ufe0f":               "❤\ufe0f",
		"می\u200cخواهم":         "می\u200cخواهم",
		"★":                     "★",
	}

	for raw, want := range accepted {
		if got, err := ParseDisplayName(raw); err != nil || got != want {
			t.Errorf("ParseDisplayName(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}

	rejected := []string{
		"", "   ", strings.Repeat("ก", 41), "line\nbreak", "tab\there", "bad \xff utf8",
		"Ann\u202eevil", "Ann\u200b", "\u2066Ann", "Ann\u2028Bob", "Ann\u2029Bob", "soft\u00adhyphen",
		"\u200d", "\u0301\u0301", "\u200c\u200d",
	}

	for _, raw := range rejected {
		if got, err := ParseDisplayName(raw); err == nil {
			t.Errorf("ParseDisplayName(%q) = %q, want an error", raw, got)
		}
	}
}

func TestNamesThatRenderBlankAreRefused(t *testing.T) {
	blanks := map[string]string{
		"hangul filler":             "\u3164",
		"halfwidth hangul filler":   "\uffa0",
		"braille pattern blank":     "\u2800",
		"hangul choseong filler":    "\u115f",
		"hangul jungseong filler":   "\u1160",
		"khmer inherent vowel":      "\u1780\u17b4",
		"combining grapheme joiner": "a\u034f",
		"filler inside a name":      "Ann\u3164Bob",
	}

	for what, raw := range blanks {
		if got, err := ParseDisplayName(raw); err == nil {
			t.Errorf("%s: ParseDisplayName(%q) = %q, want refused as invisible", what, raw, got)
		}
	}
}

func TestStackedMarksAreLimitedSoThaiPassesAndZalgoDoesNot(t *testing.T) {
	for _, ok := range []string{"ที่", "กิ๊ก", "น้ำ", "e" + strings.Repeat("\u0301", 4), "1\ufe0f\u20e3", "Tiếng Việt"} {
		if _, err := ParseDisplayName(ok); err != nil {
			t.Errorf("ParseDisplayName(%q): %v, want accepted", ok, err)
		}
	}

	for _, zalgo := range []string{
		"e" + strings.Repeat("\u0301", 5),
		"Z\u0336\u0335\u0334\u0337\u0338a",
		"e\u0301\u0301\u200d\u0301\u0301\u0301",
	} {
		if got, err := ParseDisplayName(zalgo); err == nil {
			t.Errorf("ParseDisplayName(%q) = %q, want more than 4 stacked marks refused", zalgo, got)
		}
	}
}

func TestSubdivisionFlagsAreTheOnlyPlaceForTagCharacters(t *testing.T) {
	for _, flag := range []string{england, scotland, wales, "Ann " + england, england + wales, "\U0001F3F4"} {
		if got, err := ParseDisplayName(flag); err != nil || string(got) != flag {
			t.Errorf("ParseDisplayName(%q) = %q, %v; want the flag kept", flag, got, err)
		}
	}

	for _, stray := range []string{
		"Ann\U000E0067",
		"\U000E007F",
		"\U0001F3F4\U000E007F",
		"\U0001F3F4\U000E0067\U000E0062",
		"Ann\U000E0067\U000E0062\U000E007F",
		england + "\U000E0067",
		"\U0001F3F4\U000E0001\U000E007F",
		"\U0001F3F4\U000E0069\U000E0067\U000E006E\U000E006F\U000E0072\U000E0065\U000E007F",
	} {
		if got, err := ParseDisplayName(stray); err == nil {
			t.Errorf("ParseDisplayName(%q) = %q, want tag characters outside a flag refused", stray, got)
		}
	}
}

func TestDisplayNameErrorNeverEchoesInput(t *testing.T) {
	_, err := ParseDisplayName("secret-" + strings.Repeat("x", 50))

	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v, want one that does not repeat the input", err)
	}
}

func TestAVariationSelectorMayOnlyFollowAVisibleCharacterOnce(t *testing.T) {
	for _, ok := range []string{"❤\ufe0f", "☺\ufe0e", "\u1820\u180b", "葛\U000E0100", "1\ufe0f\u20e3"} {
		if _, err := ParseDisplayName(ok); err != nil {
			t.Errorf("ParseDisplayName(%q): %v, want accepted", ok, err)
		}
	}

	for _, hidden := range []string{"Ann\ufe0f\ufe0f", "\ufe0f", "Ann \ufe0f", "e\u0301\ufe0f"} {
		if got, err := ParseDisplayName(hidden); !errors.Is(err, errNameHidden) {
			t.Errorf("ParseDisplayName(%q) = %q, %v; want errNameHidden", hidden, got, err)
		}
	}
}
