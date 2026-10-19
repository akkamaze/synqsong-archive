package room

import "testing"

func TestDefaultSkipVotesIsHalf(t *testing.T) {
	if got := DefaultSettings().SkipVotes; got != 50 {
		t.Fatalf("default skip votes = %d%%, want 50%%", got)
	}
}

func TestParseSkipVotes(t *testing.T) {
	for _, n := range []int{1, 50, 100} {
		if got, err := ParseSkipVotes(n); err != nil || int(got) != n {
			t.Errorf("ParseSkipVotes(%d) = %d, %v; want %d", n, got, err, n)
		}
	}

	for _, n := range []int{0, -1, 101} {
		if got, err := ParseSkipVotes(n); err == nil {
			t.Errorf("ParseSkipVotes(%d) = %d, want an error", n, got)
		}
	}
}

func TestVotesNeeded(t *testing.T) {
	cases := []struct {
		percent  SkipVotes
		eligible int
		want     int
	}{
		{50, 4, 2},
		{50, 5, 3},
		{50, 1, 1},
		{50, 0, 1},
		{100, 7, 7},
		{1, 49, 1},
		{34, 3, 2},
	}

	for _, c := range cases {
		if got := c.percent.VotesNeeded(c.eligible); got != c.want {
			t.Errorf("%d%% of %d eligible voters needs %d votes, want %d", c.percent, c.eligible, got, c.want)
		}
	}
}
