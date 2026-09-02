// Package nkjv provides a reliable New King James Version downloader. Because
// the NKJV text is copyrighted (Thomas Nelson) it is not bundled in the binary;
// instead it is fetched, verse-by-verse, from an established machine-readable
// API (bolls.life GetBible), cached offline, and stored under the standard
// version archive. The app treats NKJV as a stock default alongside KJV.
package nkjv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gigatone/biblelearn/internal/books"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
	"github.com/gigatone/biblelearn/internal/textclean"
)

const baseURL = "https://bolls.life/get-text/NKJV/"

// Client is a minimal HTTP client for the GetBible NKJV API.
type Client struct {
	hc *http.Client
}

// NewClient returns a client with a modest per-request timeout so a slow or
// hanging chapter can never stall the whole install indefinitely.
func NewClient() *Client {
	return &Client{hc: &http.Client{Timeout: 20 * time.Second}}
}

// verse is one verse of JSON returned by the API.
type verse struct {
	Verse int    `json:"verse"`
	Text  string `json:"text"`
}

func (c *Client) chapter(ctx context.Context, book, chap int) ([]verse, error) {
	u := baseURL + url.PathEscape(fmt.Sprintf("%d/%d/", book, chap))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chapter %d/%d status %d", book, chap, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, err
	}
	var vs []verse
	if err := json.Unmarshal(data, &vs); err != nil {
		return nil, err
	}
	return vs, nil
}

// InstallNKJV downloads the full NKJV canon and stores it under the version.
// Each book is verified against its canonical verse count before storing; only
// books that match are persisted, so the archive holds no corrupt chapters. It
// writes a progress callback per book (book name, verses stored) and returns the
// total number of verses stored.
func InstallNKJV(ctx context.Context, s *store.Store, version model.Version, progress func(book string, verses int)) (int, error) {
	coll := version.Collection
	if coll == "" {
		coll = "bible"
	}
	if err := s.RegisterVersion(version); err != nil {
		return 0, err
	}
	client := NewClient()
	total := 0
	for _, b := range books.Books() {
		bc := model.NewBookContent(version.ID, coll, b.Ordinal)
		bc.BookName = b.Name
		for chap := 1; chap <= b.Chapters; chap++ {
			vs, err := client.chapterWithRetry(ctx, b.Ordinal, chap)
			if err != nil {
				// Skip un-fetchable chapters rather than aborting the install;
				// but a book that ends up under-counted is dropped below.
				continue
			}
			if len(vs) == 0 {
				continue
			}
			ch := &model.Chapter{
				VersionID:  version.ID,
				Collection: coll,
				Book:       b.Ordinal,
				Number:     chap,
				Verses:     map[int]string{},
				Order:      []int{},
			}
			for _, v := range vs {
				if v.Verse < 1 {
					continue
				}
				text := textclean.HTML(v.Text)
				if _, ok := ch.Verses[v.Verse]; !ok {
					ch.Order = append(ch.Order, v.Verse)
				}
				ch.Verses[v.Verse] = text
			}
			if len(ch.Verses) > 0 {
				bc.Chapters[chap] = ch
				bc.ChapterOrder = append(bc.ChapterOrder, chap)
			}
		}
		n := verseCount(bc)
		// Only store books that match the canonical verse count, so corrupt or
		// partially-fetched books never reach the archive.
		if len(bc.Chapters) == 0 || n != books.VerseCount(b.Ordinal) {
			continue
		}
		if err := s.PutBook(version.ID, coll, b.Ordinal, bc); err != nil {
			return total, err
		}
		if err := s.IndexBook(version.ID, coll, b.Ordinal); err != nil {
			return total, err
		}
		total += n
		if progress != nil {
			progress(b.Name, n)
		}
	}
	return total, nil
}

func verseCount(bc *model.BookContent) int {
	n := 0
	for _, ch := range bc.Chapters {
		if ch != nil {
			n += len(ch.Verses)
		}
	}
	return n
}

// chapterWithRetry fetches a chapter, retrying a few times across transient
// errors so a flaky request does not silently drop a chapter.
func (c *Client) chapterWithRetry(ctx context.Context, book, chap int) ([]verse, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		vs, err := c.chapter(ctx, book, chap)
		if err == nil {
			return vs, nil
		}
		last = err
	}
	return nil, last
}
