#!/usr/bin/env bash
# Builds the app for the current OS, ready to distribute, into dist/:
#
#   - Windows: wgen-<version>-windows.zip, with wgen.exe (the app, no
#     console window, with its icon) and wgen-cli.exe (for the command line)
#   - macOS: wgen-<version>-macos.zip, with wgen.app (universal: Apple
#     silicon and Intel)
#   - Linux: wgen-<version>-linux.tar.gz, with the wgen program
#
# Each has an example project. Usage: scripts/package.sh [version]
#
# Needs Go and a C compiler (cgo); on Windows, go-winres for the icon
# (go install github.com/tc-hib/go-winres@latest); on Linux, the X11 and
# OpenGL development packages (see .github/workflows/build.yml).
set -euo pipefail

version=${1:-dev}
cd "$(dirname "$0")/.."
root=$PWD
ldflags="-s -w -X main.version=$version"

case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN* | Windows_NT) os=windows ;;
Darwin) os=macos ;;
*) os=linux ;;
esac

name=wgen-$version-$os
out=dist/$name
rm -rf "$out"
mkdir -p "$out"

# The example project and the readme, next to the app
examples() {
	mkdir -p "$1/example"
	cp lab/chasers.json lab/chasers.png "$1/example/"
	cp README.md "$1/"
}

case $os in
windows)
	# The icon and the version, as a resource linked in the program
	(cd cmd/wgen && go-winres simply --icon "$root/assets/icon.png" --manifest gui \
		--product-name wgen --file-description "wgen: painted maps to landscapes" \
		--product-version "$version" --file-version "$version" --arch amd64)
	trap 'rm -f cmd/wgen/rsrc_windows_*.syso' EXIT
	go build -trimpath -ldflags "$ldflags -H=windowsgui" -o "$out/wgen.exe" ./cmd/wgen
	go build -trimpath -ldflags "$ldflags" -o "$out/wgen-cli.exe" ./cmd/wgen
	examples "$out"
	(cd dist && rm -f "$name.zip" && powershell -NoProfile -Command \
		"Compress-Archive -Path '$name\\*' -DestinationPath '$name.zip'")
	;;

macos)
	export MACOSX_DEPLOYMENT_TARGET=11.0
	export CGO_ENABLED=1
	for arch in arm64 amd64; do
		clang_arch=$arch
		[ "$arch" = amd64 ] && clang_arch=x86_64
		GOARCH=$arch CC="clang -arch $clang_arch" \
			go build -trimpath -ldflags "$ldflags" -o "$out/wgen-$arch" ./cmd/wgen
	done

	app=$out/wgen.app
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
	lipo -create -output "$app/Contents/MacOS/wgen" "$out/wgen-arm64" "$out/wgen-amd64"
	rm "$out/wgen-arm64" "$out/wgen-amd64"

	# The icon, at every size macOS wants
	iconset=$out/wgen.iconset
	mkdir -p "$iconset"
	for size in 16 32 128 256 512; do
		sips -z $size $size assets/icon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
		double=$((size * 2))
		[ $double -le 512 ] && sips -z $double $double assets/icon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
	done
	cp assets/icon.png "$iconset/icon_512x512@2x.png"
	iconutil -c icns -o "$app/Contents/Resources/wgen.icns" "$iconset"
	rm -r "$iconset"

	sed "s/@VERSION@/${version#v}/g" scripts/Info.plist >"$app/Contents/Info.plist"
	# Not notarized: signed ad hoc, which Apple silicon needs to run it
	codesign --force --deep --sign - "$app"

	examples "$out"
	(cd dist && rm -f "$name.zip" && ditto -c -k --keepParent "$name" "$name.zip")
	;;

linux)
	go build -trimpath -ldflags "$ldflags" -o "$out/wgen" ./cmd/wgen
	examples "$out"
	tar -C dist -czf "dist/$name.tar.gz" "$name"
	;;
esac

echo "Built dist/$name"
