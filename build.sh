#!/bin/sh
set -eu

usage() {
	cat <<'EOF'
Usage: ./build.sh --firefox firefox-VERSION.tar.xz [--version X.Y.Z] [--skip-tests]

Builds Keycat for Linux with the Firefox for Linux given (the .tar.xz from
archive.mozilla.org) as its engine: ready to run in dist/linux/keycat, and
packed into dist/keycat-linux-x86_64.tar.gz.
Needs Go; node, when there is one, checks the stripped JavaScript.
EOF
}

version=1.3.0
firefox=
tests=yes
while [ $# -gt 0 ]; do
	case $1 in
	--firefox)
		firefox=${2:?}
		shift 2
		;;
	--version)
		version=${2:?}
		shift 2
		;;
	--skip-tests)
		tests=no
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		exit 2
		;;
	esac
done
if [ -z "$firefox" ] || [ ! -f "$firefox" ]; then
	usage >&2
	exit 2
fi
firefox=$(cd "$(dirname "$firefox")" && pwd)/$(basename "$firefox")

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"
export GOFLAGS=-mod=mod CGO_ENABLED=0

if [ "$tests" = yes ]; then
	go vet ./...
	go test ./internal/... ./tools/...
fi

stage=$root/build/stage
rm -rf "$stage"
mkdir -p "$stage"
cp -R go.mod go.sum assets.go cmd internal tools "$stage/"
go run ./tools/strip -src hub -dst "$stage/hub"
go run ./tools/strip -src browser -dst "$stage/browser"
if command -v node >/dev/null 2>&1; then
	node tools/check-js.mjs "$stage/hub" "$stage/browser"
else
	echo "node not found; the stripped JavaScript was not checked" >&2
fi

cd "$stage"
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$root/build/linux/keycat" ./cmd/keycat
go run ./tools/mktarball -firefox "$firefox" -keycat "$root/build/linux/keycat" -root "$root" \
	-dir "$root/dist/linux/keycat" -out "$root/dist/keycat-linux-x86_64.tar.gz"
echo "ready to run: $root/dist/linux/keycat/keycat"
echo "linux package: $root/dist/keycat-linux-x86_64.tar.gz"
