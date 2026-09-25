# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- Run (single URL): `go run main.go -url=http://127.0.0.1:80/test_file -targetPath=./`
- Run (links file, one URL per line, blank lines and `#` comments ignored): `go run main.go -f=links.txt -targetPath=./`
- Test: `go test ./... -race`
- Single test: `go test ./downloader -run TestDownloadAll_PartialFailure -v`
- Lint: `gofmt -l . && go vet ./...`
- Manual end-to-end: `bin/lets-download.sh [url targetPath]` creates a 500MB zero file, serves it with an nginx Docker container on port 80, runs the app, then cleans up. It uses `../main.go`, so run it from `bin/`.

Module name is `let_s_download` (Go 1.21, standard library only).

## Architecture

A CLI that downloads files by splitting each one into byte ranges fetched in parallel.

- `main.go`: parses flags and validates `-targetPath`. `-f` overrides `-url`. Builds each download with `downloader.NewDownload` (it requires an http/https URL and a resource name). Batch mode calls `downloader.DownloadAll` with at most `maxParallelDownloads` running at once.
- `downloader/download.go`: `Download.Do()` is the pipeline for one file:
  1. A HEAD request reads `Content-Length`.
  2. `makeSections` splits the file into byte ranges.
  3. `concurrentDownload` fetches each range with a `Range` GET, which must return 206, and streams it to a temp file.
  4. `mergeFiles` joins the sections into the output file.
  5. Temp files are removed by a deferred call, even on failure.
- `DownloadAll` runs many `Do()` calls through `runParallel` (`downloader/parallel.go`, a semaphore-limited fan-out also used for sections) and joins their errors, so one failure doesn't stop the others.
- Range planning is in `downloader/sections.go` (`byteRange`, `makeSections`). Temp files, merging and target-path validation are in `downloader/storage.go`.
- Progress messages go to `Download.Out` (nil means stdout). It must be safe for concurrent writes; tests set it to `io.Discard`.
- Requests go through the package-level `httpClient`, which has dial and response-header timeouts but deliberately no overall timeout, so large files aren't cut off.
- `downloader/parser.go`: `ExtractResourceName` prefers the `filename` query parameter, otherwise it uses the last part of the URL path.
- `downloader/links.go`: `ReadLinks` parses the links file.

### Non-obvious behavior
- Temp files are named `<resourceName>.section-N.tmp` in the target directory. The resource name is included so parallel downloads into one directory don't collide.
- `makeSections` always makes 10 sections and ignores `TotalSections`.
- `mergeFiles` always appends `.mp4` to the output name and opens the file with `O_APPEND` without truncating, so re-running a download appends to an existing file.
- Tests use `httptest` + `http.ServeContent`, which handles HEAD and Range requests. Test payloads must be well over 10 bytes because of the fixed 10 sections. Expected output files are `<name>.mp4`.

## Workflow

- Always ask the user before creating a git commit.
- Commit messages follow `type(scope): summary`, e.g. `feat(download): ...` or `chore(parser): ...`.
