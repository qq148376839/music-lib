package download

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
)

// fallbackOrder defines the priority order for cross-platform fallback search.
var fallbackOrder = []string{
	"kugou", "kuwo", "migu", "qq", "qianqian", "soda", "fivesing", "joox", "bilibili", "jamendo",
}

// reParens matches parenthesized or bracketed content (ASCII and CJK).
var reParens = regexp.MustCompile(`[\(（\[【][^)）\]】]*[\)）\]】]`)

// rePunctuation matches common punctuation and symbols.
var rePunctuation = regexp.MustCompile(`[.,!?;:'"、。！？；：""''·\-_]+`)

// reSpaces collapses multiple spaces into one.
var reSpaces = regexp.MustCompile(`\s+`)

// normalizeName normalises a song/artist name for fuzzy comparison:
//   - lower-case
//   - strip parenthesized content (e.g. "(Live)", "（翻唱）")
//   - strip punctuation
//   - collapse whitespace and trim
func normalizeName(s string) string {
	s = strings.ToLower(s)
	s = reParens.ReplaceAllString(s, "")
	s = rePunctuation.ReplaceAllString(s, " ")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// isSongMatch returns true when the candidate is considered the same song as the target.
//
// Rules:
//  1. Normalised names are equal AND artist contains the other (or vice versa).
//  2. One normalised name contains the other AND normalised artists are equal.
func isSongMatch(target, candidate model.Song) bool {
	tName := normalizeName(target.Name)
	cName := normalizeName(candidate.Name)
	tArtist := normalizeName(target.Artist)
	cArtist := normalizeName(candidate.Artist)

	if tName == "" || cName == "" {
		return false
	}

	artistMatch := tArtist == cArtist ||
		(tArtist != "" && cArtist != "" && (strings.Contains(tArtist, cArtist) || strings.Contains(cArtist, tArtist)))

	// Rule 1: exact name match + artist contains
	if tName == cName && artistMatch {
		return true
	}

	// Rule 2: name containment + exact artist match
	nameContains := strings.Contains(tName, cName) || strings.Contains(cName, tName)
	if nameContains && tArtist == cArtist {
		return true
	}

	return false
}

// tryFallback searches other providers for a matching song and returns a working download URL.
func (m *Manager) tryFallback(song model.Song, originalSource string) (audioURL string, fallbackSource string, err error) {
	keyword := song.Artist + " " + song.Name

	for _, name := range fallbackOrder {
		if name == originalSource {
			continue
		}
		pf, ok := m.providers[name]
		if !ok || pf.Search == nil || pf.GetDownloadURL == nil {
			continue
		}

		results, searchErr := pf.Search(keyword)
		if searchErr != nil {
			log.Printf("[download] fallback search %s error: %v", name, searchErr)
			continue
		}

		for i := range results {
			if !isSongMatch(song, results[i]) {
				continue
			}
			url, dlErr := pf.GetDownloadURL(&results[i])
			if dlErr != nil {
				log.Printf("[download] fallback download url %s error: %v", name, dlErr)
				continue
			}
			return url, name, nil
		}
	}

	return "", "", fmt.Errorf("no fallback provider found a matching download for %s", song.Display())
}
