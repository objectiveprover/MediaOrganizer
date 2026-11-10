# renamer

A terminal UI that matches movies and TV episodes against TMDB and moves them
into a Plex/Jellyfin-style library.

```
Movies/The Matrix (1999)/The Matrix (1999).mkv
Movies/Kill Bill - Vol. 1 (2003)/Kill Bill - Vol. 1 (2003) - part1.avi
TV/Dark (2017)/Season 01/Dark (2017) - S01E01 - Geheimnisse.mkv
TV/Dark (2017)/Season 01/Dark (2017) - S01E01 - Geheimnisse.en.srt
```

## Usage

```sh
export TMDB_TOKEN=...            # v4 read access token (or TMDB_API_KEY=v3 key)
go build -o renamer ./cmd/renamer
./renamer -o /media/library /downloads
```

Credentials can also go in `~/.config/renamer/config.toml`:

```toml
tmdb_token = "eyJ..."
```

Flags: `-o/--out` library root (required), `--workers` concurrent lookups
(default 8), `--no-cache` skip the response cache in `~/.cache/renamer/tmdb` (7-day TTL).

## Workflow

1. **Scan.** Finds videos and subtitles. Samples, `Extras`/`Featurettes`-style folders and
   hidden files are ignored (press `i` to see them).
2. **Match.** Groups files (all episodes of a show share one lookup) and searches TMDB.
3. **Review.** Every file shows its source and target path:

   | icon | meaning | selected by default |
   |---|---|---|
   | `✓` | confident (blue = chosen by you) | yes |
   | `~` | probably right, please verify | yes |
   | `?` | low confidence | no |
   | `✗` | no match or missing episode number | n/a |
   | `!` | conflict: target exists or two files map to the same target | skipped |

   Keys: `space` select, `x` select/deselect the whole group, `enter` fix the match,
   `e` set season/episode, `f` show only items needing attention, `/` filter, `a` apply.
4. **Fix match.** Pick another candidate, `s` search with your own text (add a year
   at the end to narrow), `i` enter a TMDB ID, `t` switch movie/TV, `u` unmatch.
   A fix applies to every file in the group.
5. **Apply.** Moves the files. It never overwrites (`renameat2(RENAME_NOREPLACE)`).
   Across filesystems it copies, fsyncs, checks the size and only then deletes the
   source. Folders left empty in the source are removed; anything else stays.

## Naming rules

- Titles use the original title when it's in Latin script, otherwise English
  (`Amélie` becomes `Le Fabuleux Destin d'Amélie Poulain`, `千と千尋の神隠し` becomes `Spirited Away`).
  Episode names follow the same rule; placeholders like "Episode 5" are omitted.
- `:` becomes ` - `, `/` becomes `-`, and characters invalid on Windows/SMB are removed.
- Subtitles keep their language as ISO 639-1 plus `.sdh`/`.forced`.

## How files are understood

The parser reads the file name first and fills gaps from parent folders, so all
of these work:

```
Breaking.Bad.S01E01.720p.HDTV.x264.mkv           S01E01, 1x01, S01E01E02, S01E01-E03
Show.Name.Complete/Season Two/01.mkv              show + season from folders
Show Name (2015)/S03/05 - Some Title.mkv
Money Heist/Temporada 1/Capitulo 3.mkv
Show.S01E02.720p.HDTV-GRP/abc123def456.mkv        obfuscated file, info in folder
Inception.2010.1080p.BluRay.x264-SPARKS/sparks-inception.mkv
```

Known gaps: anime absolute numbering (`One Piece - 1045`), date-based episodes,
and ordinal season folders like `2nd Season`.

## Development

```sh
go test ./...
TMDB_TOKEN=... go test -tags e2e ./internal/engine -run E2E -v   # real TMDB
```

| package | role |
|---|---|
| `internal/parse` | file/folder name → title, year, season, episodes |
| `internal/scan` | walk tree, ignore samples/extras, pair subtitles |
| `internal/tmdb` | API client: rate limit, retries, de-dup, disk cache |
| `internal/match` | candidate scoring and confidence |
| `internal/naming` | target paths, sanitizing, Latin-script rule |
| `internal/engine` | groups, lookups, plan, conflicts |
| `internal/fsops` | no-overwrite moves, cross-device copy |
| `internal/tui` | Bubble Tea screens |
