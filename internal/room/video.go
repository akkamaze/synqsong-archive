package room

const videoIDLen = 11

// VideoID is a YouTube video id such as "dQw4w9WgXcQ".
type VideoID string

func ParseVideoID(raw string) (VideoID, error) {
	if len(raw) != videoIDLen {
		return "", invalid("video id", "use the 11-character id from the YouTube link")
	}

	for _, c := range raw {
		if !isVideoIDChar(c) {
			return "", invalid("video id", "use the 11-character id from the YouTube link")
		}
	}

	return VideoID(raw), nil
}

func isVideoIDChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
}
