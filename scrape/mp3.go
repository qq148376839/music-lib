package scrape

import (
	"github.com/bogem/id3v2/v2"
	"github.com/guohuiyuan/music-lib/model"
)

// writeMP3Tags writes ID3v2.4 tags (title, artist, album, cover, lyrics)
// into an MP3 file. Existing tags are overwritten.
func writeMP3Tags(filePath string, song *model.Song, coverData []byte, coverMIME, lyrics string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: false})
	if err != nil {
		return err
	}
	defer tag.Close()

	tag.SetDefaultEncoding(id3v2.EncodingUTF8)
	tag.SetTitle(song.Name)
	tag.SetArtist(song.Artist)
	tag.SetAlbum(song.Album)

	// Optional fields from platform metadata.
	if year := song.Extra["year"]; year != "" {
		tag.AddTextFrame(tag.CommonID("Year"), tag.DefaultEncoding(), year)
	}
	if genre := song.Extra["genre"]; genre != "" {
		tag.SetGenre(genre)
	}

	// APIC — front cover.
	if len(coverData) > 0 {
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    coverMIME,
			PictureType: id3v2.PTFrontCover,
			Description: "Cover",
			Picture:     coverData,
		})
	}

	// USLT — unsynchronised lyrics (plain text, no LRC timestamps).
	if lyrics != "" {
		tag.AddUnsynchronisedLyricsFrame(id3v2.UnsynchronisedLyricsFrame{
			Encoding:          id3v2.EncodingUTF8,
			Language:          "zho",
			ContentDescriptor: "",
			Lyrics:            lyrics,
		})
	}

	return tag.Save()
}
