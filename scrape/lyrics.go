package scrape

import (
	"regexp"
	"strings"
)

// lrcTimestamp matches LRC timing tags: [mm:ss.xx], [mm:ss.xxx], or [mm:ss].
var lrcTimestamp = regexp.MustCompile(`\[\d{2}:\d{2}(?:\.\d{2,3})?\]`)

// StripLRCTimestamps removes LRC timing tags from lyrics text,
// returning plain text suitable for ID3v2 USLT or FLAC LYRICS embedding.
// Empty lines (after stripping) are removed.
func StripLRCTimestamps(lrc string) string {
	lines := strings.Split(lrc, "\n")
	var result []string
	for _, line := range lines {
		cleaned := lrcTimestamp.ReplaceAllString(line, "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return strings.Join(result, "\n")
}

// IsLRC returns true if the text contains at least one LRC timing tag.
func IsLRC(text string) bool {
	return lrcTimestamp.MatchString(text)
}
