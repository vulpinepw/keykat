#!/bin/sh
set -eu

usage() {
	cat <<'EOF'
Usage: ./build.sh [--firefox firefox-VERSION.tar.xz] [--android [--signing FILE]]
                  [--version X.Y.Z] [--skip-tests]

--firefox builds Keycat for Linux with the Firefox for Linux given (the
.tar.xz from archive.mozilla.org) as its engine: ready to run in
dist/linux/keycat, and packed into dist/keycat-linux-x86_64.tar.gz.

--android builds the app for Android phones into dist/keycat-android.apk.
It needs a JDK 17 or newer (JAVA_HOME) and the Android SDK with its NDK
(ANDROID_HOME); Gradle is downloaded once into build/gradle and checked
against its published checksum. --signing names the properties file with
the signing key (storeFile, storePassword, keyAlias, keyPassword); without
it the app is left unsigned.

Needs Go; node, when there is one, checks the stripped JavaScript.
EOF
}

version=1.4.0
firefox=
android=no
signing=
tests=yes
while [ $# -gt 0 ]; do
	case $1 in
	--firefox)
		firefox=${2:?}
		shift 2
		;;
	--android)
		android=yes
		shift
		;;
	--signing)
		signing=${2:?}
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
if [ -n "$firefox" ] && [ ! -f "$firefox" ]; then
	usage >&2
	exit 2
fi
if [ -z "$firefox" ] && [ "$android" = no ]; then
	usage >&2
	exit 2
fi
if [ -n "$firefox" ]; then
	firefox=$(cd "$(dirname "$firefox")" && pwd)/$(basename "$firefox")
fi
if [ -n "$signing" ]; then
	signing=$(cd "$(dirname "$signing")" && pwd)/$(basename "$signing")
fi

root=$(cd "$(dirname "$0")" && pwd)
cd "$root"
export GOFLAGS=-mod=mod CGO_ENABLED=0

if [ "$tests" = yes ]; then
	go vet ./...
	go test ./internal/... ./tools/...
	if [ "$android" = yes ]; then
		(cd android/core && GOOS=android GOARCH=arm64 go vet .)
	fi
fi

stage=$root/build/stage
rm -rf "$stage"
mkdir -p "$stage/android"
cp -R go.mod go.sum assets.go cmd internal tools "$stage/"
cp -R android/core "$stage/android/"
go run ./tools/strip -src hub -dst "$stage/hub"
go run ./tools/strip -src browser -dst "$stage/browser"
if command -v node >/dev/null 2>&1; then
	node tools/check-js.mjs "$stage/hub" "$stage/browser"
else
	echo "node not found; the stripped JavaScript was not checked" >&2
fi

if [ -n "$firefox" ]; then
	cd "$stage"
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$root/build/linux/keycat" ./cmd/keycat
	go run ./tools/mktarball -firefox "$firefox" -keycat "$root/build/linux/keycat" -root "$root" \
		-dir "$root/dist/linux/keycat" -out "$root/dist/keycat-linux-x86_64.tar.gz"
	echo "ready to run: $root/dist/linux/keycat/keycat"
	echo "linux package: $root/dist/keycat-linux-x86_64.tar.gz"
	cd "$root"
fi

if [ "$android" = yes ]; then
	sdk=${ANDROID_HOME:-$HOME/Android/Sdk}
	if [ ! -d "$sdk/platforms" ]; then
		echo "no Android SDK in $sdk; install it and set ANDROID_HOME" >&2
		exit 1
	fi
	ndk=$(ls -d "$sdk"/ndk/*/ 2>/dev/null | sort -V | tail -n 1)
	if [ -z "$ndk" ]; then
		echo "no NDK in $sdk/ndk; install one with the SDK manager" >&2
		exit 1
	fi
	if ! command -v javac >/dev/null 2>&1 && [ -z "${JAVA_HOME:-}" ]; then
		echo "no JDK found; install a JDK 17 or newer and set JAVA_HOME" >&2
		exit 1
	fi
	export ANDROID_HOME=$sdk ANDROID_NDK_HOME=${ndk%/}
	if [ -n "${JAVA_HOME:-}" ]; then
		PATH=$JAVA_HOME/bin:$PATH
	fi
	PATH=$(go env GOPATH)/bin:$PATH
	export PATH
	(
		cd "$stage/android/core"
		CGO_ENABLED=1 go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
		mkdir -p "$root/android/app/libs"
		CGO_ENABLED=1 gomobile bind -target=android/arm64 -androidapi 26 -javapkg io.github.vulpinepw.keycat \
			-trimpath -ldflags "-s -w" -o "$root/android/app/libs/keycat-core.aar" .
	)
	gradle_version=9.8.1
	gradle_sum=dce76f55f8e251a3a1f130eb120f30b3d271de2b76c9b0729d316b5a1b6dc01f
	gradle=$root/build/gradle/gradle-$gradle_version/bin/gradle
	if [ ! -x "$gradle" ]; then
		mkdir -p "$root/build/gradle"
		zip=$root/build/gradle/gradle-$gradle_version-bin.zip
		curl -fsSL -o "$zip" "https://services.gradle.org/distributions/gradle-$gradle_version-bin.zip"
		if [ "$(sha256sum "$zip" | cut -d ' ' -f 1)" != "$gradle_sum" ]; then
			rm -f "$zip"
			echo "the Gradle download does not match its checksum" >&2
			exit 1
		fi
		(cd "$root/build/gradle" && unzip -q "$zip")
		rm -f "$zip"
	fi
	set -- --no-daemon --console=plain -p "$root/android" assembleRelease -PkeycatVersion="$version" -Pabi=arm64-v8a
	name=keycat-android-unsigned.apk
	if [ -n "$signing" ]; then
		set -- "$@" -PkeycatSigning="$signing"
		name=keycat-android.apk
	fi
	"$gradle" "$@"
	mkdir -p "$root/dist"
	cp "$(ls "$root"/android/app/build/outputs/apk/release/*.apk | head -n 1)" "$root/dist/$name"
	echo "android app: $root/dist/$name"
fi
