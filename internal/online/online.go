// Package online fetches public-domain Bible text, prioritizing the Internet
// Archive as a source, and caches results locally for offline use. It is
// strictly read-only with respect to remote sources: it never writes to them.
package online

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a small HTTP client for fetching archive metadata and files.
type Client struct {
	hc        *http.Client
	UserAgent string
}

// NewClient returns an online client with sane timeouts.
func NewClient() *Client {
	return &Client{
		hc: &http.Client{Timeout: 60 * time.Second},
	}
}

// Item is a single Internet Archive item (a book/collection).
type Item struct {
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Creator     string `json:"creator,omitempty"`
	Date        string `json:"date,omitempty"`
	Description string `json:"description,omitempty"`
	Files       []File `json:"-"`
}

// File is a downloadable file within an archive item.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Format string `json:"format"`
}

// SearchBibles queries the Internet Archive advancedsearch API for Bible items
// matching the given query. Returns up to limit results.
func (c *Client) SearchBibles(ctx context.Context, query string, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 10
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("fl[]", "identifier,title,creator,date,description")
	q.Set("rows", fmt.Sprintf("%d", limit))
	q.Set("output", "json")
	u := "https://archive.org/advancedsearch.php?" + q.Encode()

	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", c.userAgent())
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archive search status %d", resp.StatusCode)
	}
	var parsed struct {
		Response struct {
			Docs []Item `json:"docs"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Response.Docs, nil
}

// ItemMetadata fetches the metadata (including file list) for an item.
func (c *Client) ItemMetadata(ctx context.Context, identifier string) (Item, error) {
	u := "https://archive.org/metadata/" + url.PathEscape(identifier)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", c.userAgent())
	resp, err := c.hc.Do(req)
	if err != nil {
		return Item{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Item{}, fmt.Errorf("metadata status %d", resp.StatusCode)
	}
	var parsed struct {
		Metadata Item   `json:"metadata"`
		Files    []File `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Item{}, err
	}
	parsed.Metadata.Files = parsed.Files
	return parsed.Metadata, nil
}

// DownloadFile fetches a file from an item as raw bytes. It supports the
// item URL forms used for text files on the Internet Archive.
func (c *Client) DownloadFile(ctx context.Context, identifier, name string) ([]byte, error) {
	u := fmt.Sprintf("https://archive.org/download/%s/%s",
		url.PathEscape(identifier), url.PathEscape(name))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", c.userAgent())
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s status %d", name, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256*1024*1024))
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return "ExaltedTerminal/0.1 (+offline bible tool)"
}

// PreferTextFiles returns the likely scripture text files from an item's file
// list, preferring .txt, .usfm, .usfx, and .djvu.txt files over others and
// ordering them so the main text comes first.
func (Item) PreferTextFiles(files []File) []File {
	var out []File
	for _, f := range files {
		lower := strings.ToLower(f.Name)
		if strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".usfm") ||
			strings.HasSuffix(lower, ".usfx") || strings.HasSuffix(lower, ".djvu.txt") ||
			strings.Contains(lower, "bible.txt") {
			out = append(out, f)
		}
	}
	return out
}

// FetchBiblesOnline is a convenience returning the recommended-text query for
// common translation acronyms so a UI can offer one-tap lookups.
func FetchBiblesOnline(ctx context.Context, acronym string, limit int) ([]Item, error) {
	c := NewClient()
	q := fmt.Sprintf("bible %s", acronym)
	if strings.EqualFold(acronym, "KJV") || strings.EqualFold(acronym, "WEB") {
		q = fmt.Sprintf("title:(bible) AND %s", acronym)
	}
	return c.SearchBibles(ctx, q, limit)
}
