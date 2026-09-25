# Let's Download
Let's Download is a simple project for concurrent file downloading.

## Quickstart

sample run
```sh
go run main.go -url=http://127.0.0.1:80/test_file -targetPath=./
```

download multiple links concurrently from a file (one link per line, blank lines and `#` comments are ignored)
```sh
go run main.go -f=links.txt -targetPath=./
```

run and test using nginx (needs Docker; the script runs `../main.go`, so run it from `bin/`)
```sh
cd bin
./lets-download.sh                     # default URL and target path
./lets-download.sh <url> <targetPath>
```

run tests
```sh
go test ./... -race
```