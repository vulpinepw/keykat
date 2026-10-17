#!/bin/sh
set -eu

usage() {
	cat <<'EOF'
Usage: uninstall.sh [--delete-data | --keep-data]

Removes Keycat, its menu entry, its icons and the keycat command.
Your profiles, passwords and settings stay in ~/.config/keycat, sealed in
their encrypted vaults, unless you add --delete-data or say so when asked.
EOF
}

delete_data=ask
for arg in "$@"; do
	case $arg in
	--delete-data) delete_data=yes ;;
	--keep-data) delete_data=no ;;
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

dest=$(cd "$(dirname "$0")" && pwd)
share=${XDG_DATA_HOME:-$HOME/.local/share}
config=${XDG_CONFIG_HOME:-$HOME/.config}/keycat
state=${XDG_STATE_HOME:-$HOME/.local/state}/keycat

if [ ! -x "$dest/keycat" ] || [ ! -x "$dest/core/firefox" ]; then
	echo "Run uninstall.sh from the folder Keycat is installed in." >&2
	exit 1
fi

if ! "$dest/keycat" --quit; then
	echo "Keycat could not be closed. Lock your open profiles, close Keycat, then run this again." >&2
	exit 1
fi

if [ "$delete_data" = ask ]; then
	delete_data=no
	if [ -t 0 ] && [ -d "$config" ]; then
		printf 'Also delete your profiles, passwords and settings in %s? This cannot be undone. [y/N] ' "$config"
		read -r answer || answer=
		case $answer in
		[yY]*) delete_data=yes ;;
		esac
	fi
fi

rm -f "$share/applications/keycat.desktop"
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database -q "$share/applications" || true
fi
for size in 16 24 32 48 64 128 256 512; do
	rm -f "$share/icons/hicolor/${size}x$size/apps/keycat.png"
done
rm -f "$share/icons/hicolor/scalable/apps/keycat.svg"
link=$HOME/.local/bin/keycat
if [ -L "$link" ] && [ "$(readlink "$link")" = "$dest/keycat" ]; then
	rm -f "$link"
fi

if [ "$delete_data" = yes ]; then
	rm -rf "$dest" "$config" "$state"
	echo "Keycat and all of its profiles were deleted."
	exit 0
fi
for item in "$dest"/* "$dest"/.[!.]*; do
	case ${item##*/} in
	Data | portable | '*' | '.[!.]*') ;;
	*) rm -rf "$item" ;;
	esac
done
rmdir "$dest" 2>/dev/null || true
echo "Keycat was removed. Your profiles were kept in $config, sealed in their encrypted vaults."
