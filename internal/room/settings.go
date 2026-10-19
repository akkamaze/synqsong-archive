package room

// Settings are the room options the owner controls.
type Settings struct {
	Skip      SkipPolicy
	SkipVotes SkipVotes
}

// SkipPolicy decides who may skip a song someone else is playing without a vote.
type SkipPolicy int8

const (
	SkipByDJs SkipPolicy = iota + 1
	SkipByEveryone
)

// SkipVotes is the percent of eligible listeners whose votes skip a song.
type SkipVotes int8

const DefaultSkipVotes SkipVotes = 50

func ParseSkipVotes(percent int) (SkipVotes, error) {
	if percent < 1 || percent > 100 {
		return 0, invalid("skip votes", "choose a percent from 1 to 100")
	}

	return SkipVotes(percent), nil
}

// VotesNeeded rounds up and never asks for fewer than one vote: 50% of 5 is 3.
func (v SkipVotes) VotesNeeded(eligible int) int {
	needed := (int(v)*eligible + 99) / 100

	return max(needed, 1)
}

func DefaultSettings() Settings {
	return Settings{Skip: SkipByDJs, SkipVotes: DefaultSkipVotes}
}
