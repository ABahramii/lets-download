# Let's Download

Let's Download is a command-line tool, written in Go, that speeds up file downloads by splitting each file into byte ranges and fetching them in parallel. It can download a single URL, or a list of URLs from a file with several downloads running at once.

Its only dependency is [mpb](https://github.com/vbauerster/mpb), which draws the progress bars.

## Features

- **Parallel range downloads.** Each file is split into 10 sections that are fetched concurrently with HTTP `Range` requests, then joined into one output file.
- **Batch mode.** Download every link in a text file, with at most 4 files downloading at the same time.
- **Isolated failures.** In batch mode, one failed download doesn't stop the others. All errors are reported together at the end.
- **One hidden temp file.** While a file downloads, all sections write straight into a single hidden `.<name>.part` file next to it. There are no per-section files and no merge step. It is renamed to `<name>` on success and removed on failure, so an existing file is never half-overwritten.
- **Progress bars.** Each file gets a coloured live bar with size, percentage, speed and ETA. The bar is split into one segment per section, so you can see each section fill at its own pace. Set `NO_COLOR` to turn colours off. Bars are shown only when stdout is a terminal. Otherwise, or with `-progress=false`, text messages are printed instead.
- **Timeouts that don't cut off large files.** Connecting and waiting for response headers time out, but there is no limit on the total download time.

## Requirements

- Go 1.21 or later
- A server that supports HTTP range requests (it must answer a `HEAD` request with `Content-Length` and answer ranged `GET` requests with `206 Partial Content`)
- Docker, only if you want to use the end-to-end test script

## Quickstart

Download a single file:

```sh
go run . -url=http://127.0.0.1:80/test_file -targetPath=./
```

Download several files listed in a text file:

```sh
go run . -f=links.txt -targetPath=./
```

You can also build a binary:

```sh
go build -o lets-download .
./lets-download -url=https://example.com/video -targetPath=./downloads
```

## Command-line flags

| Flag | Default | Description |
| --- | --- | --- |
| `-url` | `http://127.0.0.1:80/test_file` | URL of the file to download. Must be `http` or `https`. |
| `-targetPath` | current directory | Directory to save files in. It must already exist. |
| `-f` | *(empty)* | Path of a links file. When set, it overrides `-url`. |
| `-progress` | `true` | Show progress bars. Ignored when stdout is not a terminal. |

## Links file format

One URL per line. Blank lines and lines starting with `#` are ignored, and surrounding whitespace is trimmed. Lines can be up to 1 MB, so long signed URLs work.

```text
# videos
https://example.com/files/intro
https://example.com/download?id=42&filename=lecture-2

https://cdn.example.com/files/outro
```

Invalid links (bad URL, non-http scheme, or no file name) are skipped with a message, and the rest are downloaded. If no valid links remain, the program exits with status 1.

## Output file names

The file name is taken from the URL:

1. If the URL has a `filename` query parameter, its value is used (`...?filename=lecture-2` gives `lecture-2`).
2. Otherwise, the last part of the URL path is used (`.../files/intro` gives `intro`).

The output is saved as `<name>` in the target directory.

## How it works

For each file, `Download.Do()` in `downloader/download.go`:

1. Sends a `HEAD` request and reads `Content-Length` to get the file size.
2. Splits the file into 10 byte ranges (`downloader/sections.go`).
3. Creates a hidden `.<name>.part` file in the target directory and sizes it to the full file (`downloader/storage.go`).
4. Downloads every range at the same time with a `GET` request and a `Range` header. Each response must be `206 Partial Content` and is written straight into the part file at its own offset.
5. Renames the part file over the output file once every section has succeeded. A re-run replaces an existing output file. If any section fails, the part file is removed and an existing output file is left untouched.

In batch mode, `DownloadAll` runs up to 4 of these at once. Both the section downloads and the batch downloads use the same limited fan-out helper in `downloader/parallel.go`.

## Project structure

```text
.
├── main.go                  # CLI: flags, single and batch mode
├── progress.go              # mpb progress bars with per-section segments
├── downloader/
│   ├── download.go          # Download type, HTTP client, Do() and DownloadAll()
│   ├── parallel.go          # runParallel: concurrent fan-out with a limit
│   ├── progress.go          # Progress interface for reporting received bytes
│   ├── sections.go          # splitting a file into byte ranges
│   ├── storage.go           # part file, output path, target path validation
│   ├── parser.go            # getting the file name from a URL
│   ├── links.go             # reading the links file
│   └── *_test.go            # unit tests
└── bin/
    └── lets-download.sh     # end-to-end test with nginx in Docker
```

## Testing

Run all unit tests with the race detector:

```sh
go test ./... -race
```

Run a single test:

```sh
go test ./downloader -run TestDownloadAll_PartialFailure -v
```

Tests use `net/http/httptest` servers, so they don't need network access.

Check formatting and run `go vet`:

```sh
gofmt -l . && go vet ./...
```

### End-to-end test with nginx

`bin/lets-download.sh` creates a 500 MB file of zeros, serves it from an nginx Docker container on port 80, runs the downloader against it, and then cleans up. It runs the package in `..`, so start it from `bin/`:

```sh
cd bin
./lets-download.sh                       # default URL and target path
./lets-download.sh <url> <targetPath>
```

Port 80 must be free, and no other container may be named `nginx`.

## Known limitations

- Servers that don't support range requests, or that don't send `Content-Length`, are not supported.
- There is no resume support. An interrupted download has to start over.
- In single-URL mode, a failed download ends with a panic instead of a clean error message.
