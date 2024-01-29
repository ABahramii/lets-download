#!/bin/bash

GO_PROGRAM_PATH="../main.go"

# create test file for download
mkdir /tmp/nginx
dd if=/dev/zero of=/tmp/nginx/test_file bs=1M count=500

# start resource server
docker run -d \
  --name nginx \
  -p 80:80 \
  --rm \
  -v /tmp/nginx:/usr/share/nginx/html:ro \
  nginx:latest \

clear

# Check the number of arguments provided
if [ $# -eq 0 ]; then
  go run $GO_PROGRAM_PATH
elif [ $# -eq 2 ]; then
  go run $GO_PROGRAM_PATH "-url=$1" "-targetPath=$2"
else
  echo "Error: Invalid number of arguments"
  exit 1
fi

# create test file for download
rm -rf /tmp/nginx

# stop resource server
docker stop nginx > /dev/null
