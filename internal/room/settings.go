package room

// Settings are the room options the owner controls.
type Settings struct {
	Skip SkipPolicy
}

// SkipPolicy decides who may skip a song someone else is playing.
type SkipPolicy int8

const (
	SkipByDJs SkipPolicy = iota + 1
	SkipByEveryone
)

func DefaultSettings() Settings {
	return Settings{Skip: SkipByDJs}
}
