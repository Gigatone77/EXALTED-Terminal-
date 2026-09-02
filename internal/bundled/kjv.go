package bundled

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
)

// EmbeddedKJV is the public-domain King James Version USFM text, embedded
// directly in the binary so KJV works fully offline, out of the box.
//
//go:embed data/kjv.usfm.gz
var EmbeddedKJV []byte

// EmbeddedKJVUSFM returns the decompressed KJV USFM text as a string. This is
// the plug-and-play stock standard that ships with the binary.
func EmbeddedKJVUSFM() (string, error) {
	zr, err := gzip.NewReader(bytes.NewReader(EmbeddedKJV))
	if err != nil {
		return "", fmt.Errorf("open embedded KJV: %w", err)
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, zr); err != nil {
		return "", fmt.Errorf("decompress embedded KJV: %w", err)
	}
	return buf.String(), nil
}
