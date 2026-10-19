package account

import (
	"slices"
	"unicode"
)

// Thai stacks a vowel and a tone mark on one letter ("ที่"); more than 4 is Zalgo text.
const maxStackedMarks = 4

const (
	zwnj = '\u200c'
	zwj  = '\u200d'
	// Glyphs that are drawn empty but are not marked ignorable in Unicode.
	braillePatternBlank = '\u2800'
)

// The only emoji built from tag characters: the England, Scotland and Wales flags. Tags
// anywhere else could carry hidden text.
var subdivisionFlags = [][]rune{
	[]rune("\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F"),
	[]rune("\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F"),
	[]rune("\U0001F3F4\U000E0067\U000E0062\U000E0077\U000E006C\U000E0073\U000E007F"),
}

func checkCharacters(cs []rune) error {
	visible := false
	marks := 0
	// A variation selector picks how the character before it is drawn, so it needs one, once.
	selectable := false

	for i := 0; i < len(cs); i++ {
		c := cs[i]

		if n := subdivisionFlagAt(cs, i); n > 0 {
			i += n - 1
			visible = true
			marks = 0
			selectable = false

			continue
		}

		if unicode.Is(unicode.Variation_Selector, c) {
			if !selectable {
				return errNameHidden
			}

			selectable = false

			continue
		}

		selectable = unicode.In(c, unicode.L, unicode.N, unicode.P, unicode.S)

		if isHidden(c) {
			return errNameHidden
		}

		switch {
		case unicode.In(c, unicode.Mn, unicode.Me):
			marks++
		case c != zwnj && c != zwj:
			marks = 0
		}

		if marks > maxStackedMarks {
			return errNameStacked
		}

		if unicode.In(c, unicode.L, unicode.N, unicode.P, unicode.S) {
			visible = true
		}
	}

	if !visible {
		return errNameNoVisible
	}

	return nil
}

// subdivisionFlagAt returns the length of the flag starting at i, or 0 when none does.
func subdivisionFlagAt(cs []rune, i int) int {
	for _, flag := range subdivisionFlags {
		if len(cs)-i >= len(flag) && slices.Equal(cs[i:i+len(flag)], flag) {
			return len(flag)
		}
	}

	return 0
}

// Joiners are invisible, but scripts and emoji sequences need them.
func isHidden(c rune) bool {
	if c == zwnj || c == zwj {
		return false
	}

	return c == braillePatternBlank || unicode.In(c, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Other_Default_Ignorable_Code_Point)
}
