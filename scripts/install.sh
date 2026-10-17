#!/bin/sh
# Build cali for this machine and install it on PATH.
# Works on macOS, Linux, WSL (Ubuntu) and Windows Git Bash.
#   sh scripts/install.sh              build and install
#   sh scripts/install.sh --uninstall  remove the installed binary
#   BINDIR=/some/dir sh scripts/install.sh   install somewhere else
# POSIX sh (dash on Ubuntu has no pipefail), so there are no pipelines below.
set -eu

die() { printf 'install: %s\n' "$*" >&2; exit 1; }
say() { printf '%s\n' "$*"; }

cd "$(dirname "$0")/.."

case "$(uname -s)" in
	Darwin) goos=darwin ;;
	Linux) goos=linux ;;
	MINGW* | MSYS* | CYGWIN*) goos=windows ;;
	*) die "unsupported OS: $(uname -s)" ;;
esac
case "$(uname -m)" in
	x86_64 | amd64) goarch=amd64 ;;
	arm64 | aarch64) goarch=arm64 ;;
	*) die "unsupported CPU: $(uname -m)" ;;
esac

ext=
platform=$goos
if [ "$goos" = windows ]; then
	ext=.exe
	platform="Windows (Git Bash)"
elif [ "$goos" = linux ] && grep -qi microsoft /proc/version 2>/dev/null; then
	platform="WSL (Linux side only; Windows needs its own install from Git Bash)"
fi

if [ "$goos" = windows ]; then
	bindir=${BINDIR:-$HOME/bin}
else
	bindir=${BINDIR:-$HOME/.local/bin}
fi
target="$bindir/cali$ext"

if [ "${1:-}" = --uninstall ]; then
	if [ -e "$target" ]; then
		rm -f "$target"
		say "removed $target"
	else
		say "nothing to remove at $target"
	fi
	say "skills, cali.yaml, cali.lock and Cali's config/data/cache dirs were left alone"
	exit 0
fi

command -v git >/dev/null 2>&1 || die "git is not on PATH; Cali needs it to fetch skills"
command -v go >/dev/null 2>&1 || die "go is not on PATH; install Go 1.27 or later from https://go.dev/dl/"

say "platform: $platform ($goos/$goarch)"
say "building bin/cali$ext"
CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -o "bin/cali$ext" ./cmd/cali

mkdir -p "$bindir"
cp "bin/cali$ext" "$target"
[ "$goos" = windows ] || chmod 0755 "$target"
say "installed $target"

case ":$PATH:" in
	*":$bindir:"*) ;;
	*)
		case "${SHELL:-}" in
			*zsh) rc="$HOME/.zshrc" ;;
			*) rc="$HOME/.bashrc" ;;
		esac
		line="export PATH=\"$bindir:\$PATH\""
		if [ -L "$rc" ]; then
			# A symlinked rc is managed elsewhere (a dotfiles repo): do not edit it.
			say "$bindir is not on PATH. $rc is a symlink, so add this line where it is managed:"
			say "  $line"
		elif [ -f "$rc" ] && grep -qF "$line" "$rc"; then
			say "$rc already adds $bindir to PATH; open a new terminal"
		else
			printf '\n# added by cali install\n%s\n' "$line" >>"$rc"
			say "added $bindir to PATH in $rc; open a new terminal"
		fi
		;;
esac

"$target" --version
say "done. Next: cali init (project) or cali sync -g (global skills). See SETUP.md."
