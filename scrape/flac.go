package scrape

import (
	"fmt"

	flac "github.com/go-flac/go-flac"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/flacvorbis"
	"github.com/guohuiyuan/music-lib/model"
)

// writeFLACTags writes Vorbis Comment tags and an optional PICTURE block
// into a FLAC file. Any existing Vorbis Comment block is replaced entirely.
func writeFLACTags(filePath string, song *model.Song, coverData []byte, coverMIME, lyrics string) error {
	f, err := flac.ParseFile(filePath)
	if err != nil {
		return fmt.Errorf("parse flac: %w", err)
	}

	// Build a fresh Vorbis Comment block.
	cmts := flacvorbis.New()
	cmts.Add(flacvorbis.FIELD_TITLE, song.Name)
	cmts.Add(flacvorbis.FIELD_ARTIST, song.Artist)
	cmts.Add(flacvorbis.FIELD_ALBUM, song.Album)

	if year := song.Extra["year"]; year != "" {
		cmts.Add(flacvorbis.FIELD_DATE, year)
	}
	if genre := song.Extra["genre"]; genre != "" {
		cmts.Add(flacvorbis.FIELD_GENRE, genre)
	}
	if lyrics != "" {
		cmts.Add("LYRICS", lyrics)
	}

	cmtBlock := cmts.Marshal()

	// Replace existing Vorbis Comment block or append a new one.
	replaced := false
	for i, block := range f.Meta {
		if block.Type == flac.VorbisComment {
			f.Meta[i] = &cmtBlock
			replaced = true
			break
		}
	}
	if !replaced {
		f.Meta = append(f.Meta, &cmtBlock)
	}

	// Embed cover art as a PICTURE metadata block.
	if len(coverData) > 0 {
		pic, err := flacpicture.NewFromImageData(
			flacpicture.PictureTypeFrontCover,
			"Cover",
			coverData,
			coverMIME,
		)
		if err == nil {
			picBlock := pic.Marshal()
			f.Meta = append(f.Meta, &picBlock)
		}
	}

	return f.Save(filePath)
}
