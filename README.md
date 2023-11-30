# Foliage CLI

## Crosscompile
docker run -it --rm \
  -v ./:/go/src/github.com/foliagecp/cli \
  -w /go/src/github.com/foliagecp/cli \
  -e CGO_ENABLED=1 \
  docker.elastic.co/beats-dev/golang-crossbuild:1.21.1-main \
  --build-cmd "go build -o foliage-cli *.go" \
  -p "linux/amd64"

https://github.com/elastic/golang-crossbuild