#!/bin/sh
set -eu

usage() {
	cat <<'EOF'
Usage: ./install.sh [--no-launch]

Installs Keycat for your user account, or updates the Keycat already there:
  the program goes into ~/.local/share/keycat
  the menu entry and icons into ~/.local/share
  the keycat command into ~/.local/bin

Your profiles stay in ~/.config/keycat; updating does not change them.
Keycat opens when it is installed, unless you add --no-launch.
To make it your default browser, choose Make default in Keycat's settings,
or run: xdg-settings set default-web-browser keycat.desktop
To remove it, run ~/.local/share/keycat/uninstall.sh
EOF
}

launch=yes
for arg in "$@"; do
	case $arg in
	--no-launch) launch=no ;;
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

if [ "$(id -u)" -eq 0 ]; then
	echo "Run this as yourself, not as root: Keycat is installed for one user." >&2
	exit 1
fi

src=$(cd "$(dirname "$0")" && pwd)
share=${XDG_DATA_HOME:-$HOME/.local/share}
dest=$share/keycat
bin=$HOME/.local/bin

if [ ! -x "$src/keycat" ] || [ ! -x "$src/core/firefox" ]; then
	echo "Run install.sh from the folder Keycat was unpacked into." >&2
	exit 1
fi
if [ "$src" = "$dest" ]; then
	echo "This copy of Keycat is the installed one already." >&2
	exit 1
fi
case $dest in
*[\"\$\`\\]*)
	echo "Keycat cannot be installed into $dest: its path contains a quote, \$, \` or \\." >&2
	exit 1
	;;
esac

if ! "$src/keycat" --quit; then
	echo "Keycat could not be closed. Lock your open profiles, close Keycat, then run this again." >&2
	exit 1
fi

version=$("$src/keycat" --version 2>/dev/null || true)
echo "Installing Keycat $version into $dest"
mkdir -p "$share"
rm -rf "$dest.new"
mkdir "$dest.new"
for item in keycat core icons uninstall.sh LICENSE.txt THIRD-PARTY-NOTICES.txt; do
	cp -R -p "$src/$item" "$dest.new/"
done
if [ -e "$dest" ]; then
	rm -rf "$dest.old"
	mv "$dest" "$dest.old"
	for keep in Data portable; do
		if [ -e "$dest.old/$keep" ]; then
			mv "$dest.old/$keep" "$dest.new/"
		fi
	done
fi
mv "$dest.new" "$dest"
rm -rf "$dest.old"

program=$dest/keycat
case $program in
*[[:space:]]*) program="\"$program\"" ;;
esac
mkdir -p "$share/applications"
cat >"$share/applications/keycat.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Keycat
GenericName=Web Browser
Comment=Browser profiles sealed in encrypted vaults
Exec=$program %u
TryExec=$dest/keycat
Icon=keycat
Terminal=false
Categories=Network;WebBrowser;
MimeType=text/html;application/xhtml+xml;x-scheme-handler/http;x-scheme-handler/https;
Keywords=browser;privacy;profiles;vault;passwords;
StartupNotify=true
StartupWMClass=Keycat
EOF
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database -q "$share/applications" || true
fi

for size in 16 24 32 48 64 128 256 512; do
	mkdir -p "$share/icons/hicolor/${size}x$size/apps"
	cp "$dest/icons/keycat-$size.png" "$share/icons/hicolor/${size}x$size/apps/keycat.png"
done
mkdir -p "$share/icons/hicolor/scalable/apps"
cp "$dest/icons/keycat.svg" "$share/icons/hicolor/scalable/apps/keycat.svg"
touch "$share/icons/hicolor"

mkdir -p "$bin"
if [ -e "$bin/keycat" ] && [ ! -L "$bin/keycat" ]; then
	echo "Left $bin/keycat alone: it is not Keycat's."
else
	ln -sf "$dest/keycat" "$bin/keycat"
fi

echo "Keycat is installed. Open it from your applications menu, or run: keycat"
case ":$PATH:" in
*":$bin:"*) ;;
*) echo "($bin is not in your PATH, so the keycat command needs its full path there.)" ;;
esac

if [ "$launch" = yes ] && { [ -n "${WAYLAND_DISPLAY:-}" ] || [ -n "${DISPLAY:-}" ]; }; then
	nohup "$dest/keycat" >/dev/null 2>&1 &
fi
