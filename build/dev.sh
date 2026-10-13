#!/usr/bin/env bash
# `make dev`: the windows and the tray (the shell) stay up while a Go change
# rebuilds and restarts only the backend behind them — the API and the
# gateway. The page refreshes what it shows in place; UI files under
# internal/gui/assets reload the page as before. A build that fails leaves
# the running backend alone. A change to the shell's own code
# (internal/gui/app.go, dev_on.go) restarts the windows too. Quitting fogway
# from the tray, or Ctrl-C, ends it all.
#
# A restart is a handover, so an agent streaming through the dev gateway
# isn't cut off: the new backend listens on the same ports beside the old
# one, which is then told (USR2) to stop taking requests, finish those it
# has and exit. A backend that builds but never comes up leaves the old one
# serving.
#
# make passes FOGWAY_ADDR, FOGWAY_DEV_UI, FOGWAY_DEV_BACKEND and
# FOGWAY_DEV_CONTROL.
set -u
cd "$(dirname "$0")/.." || exit 1

shell='' backend='' watch='' fsw='' draining=''

stop() { [ -n "$1" ] && kill "$1" 2>/dev/null && wait "$1" 2>/dev/null; }

start_backend() { FOGWAY_DEV_ROLE=backend ./fogway-dev-backend app & backend=$!; }

# listening waits (up to 30s) for backend $1 to listen on the gateway's
# port and the backend's; no, once it has exited
listening() {
	local i
	for ((i = 0; i < 150; i++)); do
		kill -0 "$1" 2>/dev/null || return 1
		listens "$1" "${FOGWAY_ADDR##*:}" && listens "$1" "${FOGWAY_DEV_BACKEND##*:}" && return 0
		sleep 0.2
	done
	return 1
}
listens() { lsof -nP -a -p "$1" -iTCP:"$2" -sTCP:LISTEN >/dev/null 2>&1; }

# handover starts a new backend and, once it listens, lets the old one go
handover() {
	local old=$backend
	start_backend
	if listening "$backend"; then
		kill -USR2 "$old" 2>/dev/null
		draining="$draining $old"
		return
	fi
	echo "  the new backend didn't come up · the one before it keeps running"
	kill "$backend" 2>/dev/null
	wait "$backend" 2>/dev/null
	backend=$old
}

# the shell, and a watcher that ends everything once it is quit
start_shell() {
	FOGWAY_DEV_ROLE=shell ./fogway-dev app & shell=$!
	{ while kill -0 "$shell" 2>/dev/null; do sleep 1; done; kill -TERM $$; } & watch=$!
}

stop_shell() {
	kill "$watch" 2>/dev/null; wait "$watch" 2>/dev/null
	stop "$shell"
}

cleanup() {
	trap - INT TERM
	stop_shell; stop "$backend"; kill "$fsw" $draining 2>/dev/null
	exit 0
}
trap cleanup INT TERM

# what the build reads: Go, and the one Markdown file embedded
relevant() {
	case $1 in
	*/site/* | *_test.go) return 1 ;;
	*.go | */internal/agent/codex_prompt.md) return 0 ;;
	esac
	return 1
}

# builds to a new file and moves it in, never over a running binary
build() { go build -tags dev -o fogway-dev.new . && cp fogway-dev.new fogway-dev-backend.new && mv fogway-dev-backend.new fogway-dev-backend; }

build || exit 1
mv fogway-dev.new fogway-dev
start_backend
start_shell
exec 3< <(fswatch -r -e '/\.git/' -e '/site/' -e '/fogway-dev' "$PWD")
fsw=$!
echo "  fogway-dev · UI from internal/gui/assets, reload on save · Go changes restart the backend only · gateway $FOGWAY_ADDR"

while read -r -u 3 path; do
	relevant "$path" || continue
	changed=" $path"
	# let a burst of saves settle into one build
	while read -r -u 3 -t 0.3 more; do relevant "$more" && changed="$changed $more"; done
	echo "  go changed · rebuilding"
	if ! build; then
		echo "  build failed · the backend before it keeps running"
		continue
	fi
	handover
	case $changed in
	*/internal/gui/app.go* | */internal/gui/dev_on.go*)
		echo "  window code changed · reopening the windows"
		stop_shell
		mv fogway-dev.new fogway-dev
		start_shell
		;;
	*) rm -f fogway-dev.new ;;
	esac
done
