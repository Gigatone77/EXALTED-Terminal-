# EXALTED Terminal

A single-binary, offline-first Bible learning & study tool with a fast, dark-mode
terminal UI. It bundles the public-domain **KJV** and can install the **NKJV**
(and other public-domain versions) with per-book canonical verification.

Everything runs locally — no sign-up, no cloud. Your reading, memory, and notes
are stored in a local SQLite database at `~/.biblelearn`.

## Features

- **Browse** the Bible verse by verse (← → chapters, p/o books, ↑ ↓ verses)
- **Search** all installed versions with full-text matching, ranked so shorter
  books surface first
- **Memory** — spaced-repetition verse review (`s` to memorize, then grade cards)
- **Notes** — attach personal study notes to any verse
- **Offline-first** — the KJV ships inside the binary and needs no network
- **Multiple versions** — install NKJV, USFM/text, or archive-sourced Bibles
- **Church locator** plugin (open-source OpenStreetMap data)
- **Local web UI** via `serve`

## Install

Build the binary from source:

```sh
make build          # produces ./bin/exhaled
make install        # installs to ~/.local/bin/exhaled
```

The tool works offline out of the box — the embedded KJV is seeded on first run.

## Usage

Running `exhaled` with no arguments opens the **interactive terminal UI**:

```
 1 Browse    2 Search    3 Memory    4 Notes
```

- **1 Browse** — navigate chapters/verses. `c` saves a note, `s` adds the verse
  to your memory deck.
- **2 Search** — type a query; results rank shorter books first.
- **3 Memory** — spaced-repetition review: `space` reveals the answer, then grade
  with `a`/`h`/(`g`)/`e`. New verses appear here after studying them.
- **4 Notes** — browse notes you've written; `d` deletes.

Scripting / import commands:

```sh
exhaled read "John 3:16"        # print a passage (defaults to active version)
exhaled read "John 3:16 (NKJV)" # reference a specific version
exhaled search "love"           # full-text search across installed versions
exhaled versions                # list installed versions
exhaled install-nkjv            # install the New King James Version (GetBible API)
exhaled install-pdf <dir|zip>   # import a version from per-book PDF files
exhaled install-text <dir>      # import a version from USFM / plain text
exhaled fetch-archive <query>   # install a public-domain Bible from Internet Archive
exhaled notes                   # manage study notes
exhaled terms "jo"              # autocomplete terms for search
exhaled index                   # rebuild the search index
exhaled serve                   # local web/app UI
exhaled plugin list             # manage plugins (e.g. church)
exhaled church <zip>            # find nearby churches
exhaled help                    # full usage
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