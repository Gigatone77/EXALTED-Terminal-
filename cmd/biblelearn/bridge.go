package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gigatone/biblelearn/internal/collections"
	"github.com/gigatone/biblelearn/internal/engine"
	"github.com/gigatone/biblelearn/internal/model"
	"github.com/gigatone/biblelearn/internal/store"
)

// Verse-text markup left by the USFM import (Strong's codes etc.) is stripped
// so the in-game HUD shows plain readable scripture.
var (
	reStrong   = regexp.MustCompile(`\|strong="[^"]*"\*?`)
	reWordTag  = regexp.MustCompile(`\\\+w\*?`)
	reMarkdown = regexp.MustCompile(`\\[A-Za-z0-9-]*\*?`)
)

// cleanText strips USFM/word-morphology markup from stored verse text.
func cleanText(s string) string {
	s = reStrong.ReplaceAllString(s, "")
	s = reWordTag.ReplaceAllString(s, "")
	s = reMarkdown.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// bridgeVersion is the IPC protocol revision spoken by this binary.
const bridgeVersion = "0.1.0"

// bridgeResponse is a single reply written to responses.json.
type bridgeResponse struct {
	Seq   int64           `json:"seq"`
	OK    bool            `json:"ok"`
	Op    string          `json:"op"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// runBridge drives the Cyberpunk 2077 CET companion: it watches commands.json
// in a CET mod folder and answers via responses.json, using a fully offline,
// isolated copy of the EXALTED store rooted at <modfolder>/data.
func runBridge(args []string) {
	fs := flag.NewFlagSet("bridge", flag.ExitOnError)
	dir := fs.String("dir", "", "CET mod folder to watch (commands.json / responses.json)")
	dataDir := fs.String("data", "", "isolated data dir (default <dir>/data)")
	intervalMS := fs.Int("interval", 150, "poll interval in ms")
	once := fs.Bool("once", false, "single pass then exit")
	fs.Parse(args)
	if *dir == "" {
		fatal("usage: exalted bridge --dir <modfolder> [--data DIR] [--interval MS] [--once]")
	}
	modDir, err := filepath.Abs(*dir)
	must(err)
	if fi, err := os.Stat(modDir); err != nil || !fi.IsDir() {
		fatal("bridge: mod folder does not exist: " + modDir)
	}
	data := *dataDir
	if data == "" {
		data = filepath.Join(modDir, "data")
	}

	// openEngine seeds the embedded KJV idempotently and fully offline (the
	// NKJV network fetch never triggers because KJV becomes the active version).
	e, err := openEngine(data)
	must(err)
	defer e.Close()

	cmdPath := filepath.Join(modDir, "commands.json")
	respPath := filepath.Join(modDir, "responses.json")

	var lastSeq int64
	for {
		resp := handlePending(e, cmdPath, lastSeq)
		if resp != nil {
			lastSeq = resp.Seq
			if err := writeBridgeResponse(respPath, resp); err != nil {
				fmt.Fprintln(os.Stderr, "bridge: write response:", err)
			}
		}
		if *once {
			return
		}
		time.Sleep(time.Duration(*intervalMS) * time.Millisecond)
	}
}

// handlePending reads commands.json and executes the newest un-handled command.
func handlePending(e *engine.Engine, cmdPath string, lastSeq int64) *bridgeResponse {
	data, err := os.ReadFile(cmdPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return nil
	}
	var cmd struct {
		Seq  int64  `json:"seq"`
		Op   string `json:"op"`
		Args json.RawMessage
	}
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil // half-written or garbage: ignore this pass
	}
	if cmd.Seq <= lastSeq || cmd.Op == "" {
		return nil
	}
	var args map[string]any
	if len(cmd.Args) > 0 {
		_ = json.Unmarshal(cmd.Args, &args)
	}
	payload, oerr := executeOp(e, cmd.Op, args)
	if oerr != nil {
		return &bridgeResponse{Seq: cmd.Seq, OK: false, Op: cmd.Op, Error: oerr.Error()}
	}
	return &bridgeResponse{Seq: cmd.Seq, OK: true, Op: cmd.Op, Data: payload}
}

func writeBridgeResponse(respPath string, r *bridgeResponse) error {
	buf, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := respPath + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, respPath)
}

// ---------------------------------------------------------------------------
// op dispatch
// ---------------------------------------------------------------------------

func executeOp(e *engine.Engine, op string, args map[string]any) (json.RawMessage, error) {
	switch op {
	case "ping":
		return js(pingPayload(e))
	case "boot":
		p, err := bootPayload(e)
		if err != nil {
			return nil, err
		}
		return js(p)
	case "versions":
		return versionsPayload(e)
	case "setversion":
		if id, ok := argString(args, "id"); ok {
			if err := e.SetActiveVersion(id); err != nil {
				return nil, err
			}
		}
		return versionsPayload(e)
	case "books":
		v, _ := argString(args, "version")
		ver, err := e.ActiveVersionID()
		if err != nil {
			return nil, err
		}
		if v != "" {
			ver = v
		}
		return booksPayload(ver)
	case "chapter":
		return chapterPayload(e, args)
	case "read":
		return readPayload(e, args)
	case "search":
		q, _ := argString(args, "query")
		if q == "" {
			return nil, fmt.Errorf("search: query required")
		}
		limit := argInt(args, "limit", 30)
		ver, err := e.ActiveVersionID()
		if err != nil {
			return nil, err
		}
		if v, ok := argString(args, "version"); ok && v != "" {
			ver = v
		}
		results, err := e.Store.Search(q, ver, "bible", limit)
		if err != nil {
			return nil, err
		}
		type hit struct {
			Book     int    `json:"book"`
			Chapter  int    `json:"chapter"`
			Verse    int    `json:"verse"`
			BookName string `json:"bookname"`
			Text     string `json:"text"`
		}
		var out []hit
		for i := range results {
			r := &results[i]
			out = append(out, hit{r.Book, r.Chapter, r.Verse, bookName(r.Book), cleanText(r.Text)})
		}
		return js(map[string]any{"query": q, "results": out})
	case "terms":
		prefix, _ := argString(args, "prefix")
		limit := argInt(args, "limit", 20)
		ver, err := e.ActiveVersionID()
		if err != nil {
			return nil, err
		}
		if v, ok := argString(args, "version"); ok && v != "" {
			ver = v
		}
		terms, err := e.Store.SuggestTerms(prefix, ver, "bible", limit, true)
		if err != nil {
			return nil, err
		}
		type t struct {
			Term  string `json:"term"`
			Count int    `json:"count"`
		}
		var out []t
		for _, x := range terms {
			out = append(out, t{x.Term, x.Count})
		}
		return js(map[string]any{"terms": out})
	case "notes.list":
		return notesListPayload(e)
	case "notes.add":
		return notesWritePayload(e, args, false)
	case "notes.remove":
		return notesWritePayload(e, args, true)
	case "memory.add":
		ver, ref, err := refFromArgs(e, args)
		if err != nil {
			return nil, err
		}
		st := store.MemoryState{VersionID: ver, Book: ref.Book, Chapter: ref.Chapter, Verse: ref.Verse,
			IntervalDays: 0, Ease: 2.5, Reps: 0, State: 0, Due: time.Now()}
		if err := e.Store.SaveMemory(st); err != nil {
			return nil, err
		}
		return js(map[string]any{"ok": true, "ref": refString(ref)})
	case "memory.due":
		ver, err := e.ActiveVersionID()
		if err != nil {
			return nil, err
		}
		if v, ok := argString(args, "version"); ok && v != "" {
			ver = v
		}
		limit := argInt(args, "limit", 40)
		cards, err := e.Store.DueCards(ver, limit)
		if err != nil {
			return nil, err
		}
		type card struct {
			Book     int     `json:"book"`
			Chapter  int     `json:"chapter"`
			Verse    int     `json:"verse"`
			RefStr   string  `json:"refstr"`
			Text     string  `json:"text"`
			State    int     `json:"state"`
			Reps     int     `json:"reps"`
			Interval float64 `json:"interval"`
		}
		var out []card
		for _, c := range cards {
			ref := model.Ref{VersionID: ver, Collection: "bible", Book: c.Book, Chapter: c.Chapter, Verse: c.Verse}
			txt, _, _ := e.Store.VerseText(ver, ref)
			out = append(out, card{c.Book, c.Chapter, c.Verse, refString(ref), cleanText(txt), c.State, c.Reps, c.IntervalDays})
		}
		return js(map[string]any{"cards": out})
	case "memory.grade":
		ver, ref, err := refFromArgs(e, args)
		if err != nil {
			return nil, err
		}
		q := argInt(args, "quality", 4)
		c, ok, err := e.Store.GetMemory(ver, ref.Book, ref.Chapter, ref.Verse)
		if err != nil {
			return nil, err
		}
		if !ok {
			c = store.MemoryState{VersionID: ver, Book: ref.Book, Chapter: ref.Chapter, Verse: ref.Verse,
				IntervalDays: 0, Ease: 2.5, Reps: 0, State: 0, Due: time.Now()}
		}
		if err := e.Store.SaveMemory(store.ScheduleNext(c, q)); err != nil {
			return nil, err
		}
		return js(map[string]any{"ok": true})
	case "memory.stats":
		ver, err := e.ActiveVersionID()
		if err != nil {
			return nil, err
		}
		total, due, learning, review, err := e.Store.MemoryStats(ver)
		if err != nil {
			return nil, err
		}
		return js(map[string]any{"total": total, "due": due, "learning": learning, "review": review})
	case "bookmark.get":
		bm, _ := e.Store.GetSetting(bridgeBookmarkKey)
		return js(map[string]any{"ref": bm})
	case "bookmark.set":
		s, _ := argString(args, "ref")
		if err := e.Store.SetSetting(bridgeBookmarkKey, s); err != nil {
			return nil, err
		}
		return js(map[string]any{"ref": s})
	}
	return nil, fmt.Errorf("unknown op %q", op)
}

const bridgeBookmarkKey = "game.bookmark"

// ---------------------------------------------------------------------------
// payload builders
// ---------------------------------------------------------------------------

func pingPayload(e *engine.Engine) map[string]any {
	return map[string]any{
		"app":      "exalted",
		"bridge":   bridgeVersion,
		"data_dir": e.Store.Dir(),
	}
}

func bootPayload(e *engine.Engine) (map[string]any, error) {
	p, err := versionsPayload(e)
	if err != nil {
		return nil, err
	}
	var vp map[string]any
	_ = json.Unmarshal(p, &vp)
	bm, _ := e.Store.GetSetting(bridgeBookmarkKey)
	out := pingPayload(e)
	out["versions"] = vp["versions"]
	out["active"] = vp["active"]
	out["bookmark"] = bm
	return out, nil
}

func versionsPayload(e *engine.Engine) (json.RawMessage, error) {
	vs, err := e.ListVersions()
	if err != nil {
		return nil, err
	}
	active, _ := e.ActiveVersionID()
	type item struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Books    int    `json:"books"`
		Verses   int    `json:"verses"`
		Complete bool   `json:"complete"`
		Builtin  bool   `json:"builtin"`
		Active   bool   `json:"active"`
		Source   string `json:"source,omitempty"`
	}
	var out []item
	for _, v := range vs {
		if v.BooksPresent == 0 && !v.Complete {
			continue // not installed yet (e.g. registry-only)
		}
		out = append(out, item{v.ID, v.Name, v.BooksPresent, v.VersesStored, v.Complete, v.Builtin, v.ID == active, v.Source})
	}
	return js(map[string]any{"active": active, "versions": out})
}

func booksPayload(version string) (json.RawMessage, error) {
	type b struct {
		N        int    `json:"n"`
		Name     string `json:"name"`
		Chapters int    `json:"chapters"`
	}
	var out []b
	for _, bk := range collections.ByIDSafe("bible") {
		out = append(out, b{bk.Ordinal, bk.Name, bk.Chapters})
	}
	return js(map[string]any{"version": version, "books": out})
}

func chapterPayload(e *engine.Engine, args map[string]any) (json.RawMessage, error) {
	ver, _ := argString(args, "version")
	if ver == "" {
		var err error
		ver, err = e.ActiveVersionID()
		if err != nil {
			return nil, err
		}
	}
	book := argInt(args, "book", 0)
	chapter := argInt(args, "chapter", 0)
	blist := collections.ByIDSafe("bible")
	if book < 1 || book > len(blist) {
		return nil, fmt.Errorf("chapter: invalid book %d", book)
	}
	bk := blist[book-1]
	if chapter < 1 || chapter > bk.Chapters {
		return nil, fmt.Errorf("chapter: invalid chapter %d (1..%d)", chapter, bk.Chapters)
	}
	bc, ok, err := e.Store.GetBook(ver, "bible", book)
	if err != nil {
		return nil, err
	}
	type v struct {
		V    int    `json:"v"`
		Text string `json:"text"`
	}
	var verses []v
	if ok && bc.Chapters != nil {
		if ch, exists := bc.Chapters[chapter]; exists && ch != nil && ch.Verses != nil {
			for _, vn := range ch.Order {
				verses = append(verses, v{vn, cleanText(ch.Verses[vn])})
			}
		}
	}
	prev := 0
	next := 0
	if chapter > 1 {
		prev = chapter - 1
	}
	if chapter < bk.Chapters {
		next = chapter + 1
	}
	return js(map[string]any{
		"version":        ver,
		"book":           book,
		"book_name":      bk.Name,
		"chapter":        chapter,
		"total_chapters": bk.Chapters,
		"prev_chapter":   prev,
		"next_chapter":   next,
		"verses":         verses,
	})
}

func readPayload(e *engine.Engine, args map[string]any) (json.RawMessage, error) {
	ver, ref, err := refFromArgs(e, args)
	if err != nil {
		return nil, err
	}
	txt, _, err := e.Store.VerseText(ver, ref)
	if err != nil {
		return nil, err
	}
	return js(map[string]any{
		"version": ver,
		"ref":     map[string]int{"book": ref.Book, "chapter": ref.Chapter, "verse": ref.Verse},
		"refstr":  refString(ref),
		"text":    cleanText(txt),
	})
}

func notesListPayload(e *engine.Engine) (json.RawMessage, error) {
	ver, err := e.ActiveVersionID()
	if err != nil {
		return nil, err
	}
	ns, err := e.Store.ListNotes(ver)
	if err != nil {
		return nil, err
	}
	type n struct {
		Ref     string `json:"ref"`
		Book    int    `json:"book"`
		Chapter int    `json:"chapter"`
		Verse   int    `json:"verse"`
		Body    string `json:"body"`
	}
	var out []n
	for _, x := range ns {
		ref := model.Ref{VersionID: ver, Collection: "bible", Book: x.Book, Chapter: x.Chapter, Verse: x.Verse}
		out = append(out, n{refString(ref), x.Book, x.Chapter, x.Verse, x.Body})
	}
	return js(map[string]any{"notes": out})
}

func notesWritePayload(e *engine.Engine, args map[string]any, remove bool) (json.RawMessage, error) {
	ver, ref, err := refFromArgs(e, args)
	if err != nil {
		return nil, err
	}
	var body string
	if !remove {
		body, _ = argString(args, "body")
		if body == "" {
			return nil, fmt.Errorf("notes.add: body required")
		}
	}
	if err := e.Store.SaveNote(store.Note{VersionID: ver, Book: ref.Book, Chapter: ref.Chapter, Verse: ref.Verse, Body: body}); err != nil {
		return nil, err
	}
	return js(map[string]any{"ok": true, "ref": refString(ref)})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func refFromArgs(e *engine.Engine, args map[string]any) (string, model.Ref, error) {
	ver, _ := argString(args, "version")
	if ver == "" {
		var err error
		ver, err = e.ActiveVersionID()
		if err != nil {
			return "", model.Ref{}, err
		}
	}
	refstr, _ := argString(args, "ref")
	r, ok := model.ParseRef(refstr)
	if !ok {
		return "", model.Ref{}, fmt.Errorf("bad reference %q", refstr)
	}
	r.VersionID = ver
	r.Collection = "bible"
	return ver, r, nil
}

func refString(r model.Ref) string {
	if r.Verse > 0 {
		return fmt.Sprintf("%s %d:%d", r.BookName(), r.Chapter, r.Verse)
	}
	return fmt.Sprintf("%s %d", r.BookName(), r.Chapter)
}

// bookName resolves a canonical book ordinal to its display name.
func bookName(ord int) string {
	list := collections.ByIDSafe("bible")
	if ord >= 1 && ord <= len(list) {
		return list[ord-1].Name
	}
	return "?"
}

func js(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func argString(m map[string]any, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func argInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		var n int
		if _, err := fmt.Sscan(t, &n); err == nil {
			return n
		}
	}
	return def
}
