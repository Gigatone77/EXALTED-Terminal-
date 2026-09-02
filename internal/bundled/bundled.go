// Package bundled defines the known, reliable public-domain Bible sources and
// the out-of-the-box setup routine. The app boots offline-first: if no local
// version has text, it can seed from a known Internet Archive item.
package bundled

import "github.com/gigatone/biblelearn/internal/model"

// Source describes a known downloadable Bible source.
type Source struct {
	// ID is the version ID to install under (e.g. "KJV").
	ID string
	// Name of the translation.
	Name string
	// Lang BCP-47 tag.
	Lang string
	// ArchiveID is the Internet Archive item identifier.
	ArchiveID string
	// Source describes how the version is obtained (e.g. "Internet Archive" or
	// "GetBible"). Empty defaults to "Internet Archive".
	Source string
	// Builtin indicates a version bundled directly in the binary (none today).
	Builtin bool
}

// Sources lists the stock standard translations the app ships with by default.
// KJV is the public-domain bundled standard; NKJV is the dominant copyrighted
// translation pulled from the reliable GetBible API. Both are treated as stock
// defaults. Arranged by preference (NKJV first as the dominant default, with
// KJV as the public-domain fallback), followed by other public-domain options.
var Sources = []Source{
	{ID: "NKJV", Name: "New King James Version", Lang: "en", Source: "GetBible", ArchiveID: ""},
	{ID: "KJV", Name: "King James Version", Lang: "en", ArchiveID: "The_Holy_Bible_KJV"},
	{ID: "WEB", Name: "World English Bible", Lang: "en", ArchiveID: "worldenglishbible"},
	{ID: "ASV", Name: "American Standard Version", Lang: "en", ArchiveID: "asvbible"},
	{ID: "DARBY", Name: "Darby Bible", Lang: "en", ArchiveID: "darbybible"},
	{ID: "YLT", Name: "Young's Literal Translation", Lang: "en", ArchiveID: "yltbible"},
}

// ByID returns the source for a version ID, if any.
func ByID(id string) (Source, bool) {
	for _, s := range Sources {
		if s.ID == id {
			return s, true
		}
	}
	return Source{}, false
}

// Version converts a source to version metadata.
func (s Source) Version() model.Version {
	return model.Version{
		ID:        s.ID,
		Name:      s.Name,
		Lang:      s.Lang,
		Source:    "Internet Archive",
		SourceURL: "https://archive.org/details/" + s.ArchiveID,
		Builtin:   s.Builtin,
		Enabled:   true,
	}
}
