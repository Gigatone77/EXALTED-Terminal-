# EXALTED Terminal

Two editions of one offline-first Bible learning & study tool:

- **Terminal** — a fast, dark-mode TUI (`exalted`)
- **Graphical** — a modern Fyne desktop app (`exalted-gui`), also packaged as a
  portable **AppImage** you can double-click

Both bundle the public-domain **KJV** and can install the **NKJV** (and other
public-domain versions) with per-book canonical verification. Everything runs
locally — no sign-up, no cloud. Your reading, memory, and notes are stored in a
local SQLite database at `~/.biblelearn`.

## Features

- **Browse** the Bible verse by verse — navigate chapters/verses, and expand the
  view to show a passage of several consecutive verses at once
- **Search** all installed versions with full-text matching, ranked so shorter
  books surface first
- **Memory** — spaced-repetition verse review (add a verse, then grade cards)
- **Notes** — attach personal study notes to any verse
- **Offline-first** — the KJV ships inside the binary and needs no network
- **Multiple versions** — install NKJV, USFM/text, or archive-sourced Bibles
- **Church locator** plugin (open-source OpenStreetMap data)

## Install

### Graphical AppImage (no build required)

Download/extract the portable AppImage (a single `EXALTED_Terminal-x86_64.AppImage`
file, bundles its own GL/Wayland libs), make it executable, and run:

```sh
chmod +x EXALTED_Terminal-x86_64.AppImage
./EXALTED_Terminal-x86_64.AppImage
```

### Build from source

The terminal edition needs only the Go toolchain:

```sh
make build          # produces ./bin/exalted
make install        # installs to ~/.local/bin/exalted
```

The graphical edition needs GL/Wayland/X11 dev headers (via Homebrew on Fedora
Atomic) and `appimagetool`:

```sh
make gui            # build ./bin/exalted-gui
make appimage       # build dist/EXALTED_Terminal-x86_64.AppImage
```

The tool works offline out of the box — the embedded KJV is seeded on first run.

## Usage

Running `exalted` with no arguments opens the **interactive terminal UI**:

```
 1 Browse    2 Search    3 Memory    4 Notes
```

- **1 Browse** — navigate chapters/verses. `]` expands to show several
  consecutive verses at once, `[` shrinks back, `0` resets. `c` saves a note,
  `s` adds the verse to your memory deck.
- **2 Search** — type a query; results rank shorter books first.
- **3 Memory** — spaced-repetition review: `space` reveals the answer, then grade
  with `a`/`h`/(`g`)/`e`. New verses appear here after studying them.
- **4 Notes** — browse notes you've written; `d` deletes.

### Graphical app

Launching `exalted-gui` (or the AppImage) opens the desktop window with the
same four areas as tabs: **Browse** (book/chapter navigation on the left, the
passage on the right), **Search**, **Memory**, and **Notes**. It reads and writes
the same `~/.biblelearn` data as the terminal edition.

Scripting / import commands:

```sh
exalted read "John 3:16"        # print a passage (defaults to active version)
exalted read "John 3:16 (NKJV)" # reference a specific version
exalted search "love"           # full-text search across installed versions
exalted versions                # list installed versions
exalted install-nkjv            # install the New King James Version (GetBible API)
exalted install-pdf <dir|zip>   # import a version from per-book PDF files
exalted install-text <dir>      # import a version from USFM / plain text
exalted fetch-archive <query>   # install a public-domain Bible from Internet Archive
exalted notes                   # manage study notes
exalted terms "jo"              # autocomplete terms for search
exalted index                   # rebuild the search index
exalted serve                   # local web/app UI
exalted plugin list             # manage plugins (e.g. church)
exalted church <zip>            # find nearby churches
exalted help                    # full usage
```

### Stock versions

- **KJV** — embedded in the binary, seeded offline on first run (public domain).
- **NKJV** — fetched from the GetBible API with per-book canonical verse-count
  verification before storage; `install-nkjv` makes it the active default.

### Data directory

Data lives in `~/.biblelearn` (override with `--data DIR`). The bundled KJV is
seeded idempotently (it will not overwrite existing data).

## Development

```sh
make tidy   # gofmt + go mod tidy
make test   # run the test suite
go vet ./... # static checks
```

## License

The tool is free to use. Biblical text sources carry their own rights: the KJV
is public domain; the NKJV is © Thomas Nelson and is provided here for personal
study, not redistribution.