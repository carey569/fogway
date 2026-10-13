package gateway

// unloadable stands in for Claude Code given --resume on a session it
// can't load: it says so and exits 1, as Claude Code 2.1.295 does ("No
// conversation found with session ID"). It reads one line first, so the
// line fogway wrote is taken and fogway hears of the end only as the
// process goes: the order in which the turn was answered 502 "Claude Code
// ended", with no reason (FrierenF on Discord).
const unloadable = `if [ -n "$file" ] && [ "$sid" != "s$$" ] && [ ! -s "$file" ]; then
  read -r line
  echo "No conversation found with session ID: $sid" >&2
  exit 1
fi`

// safeguards is what Claude Code in auto mode sends with each turn; a
// run is told it with a control request before the turn (setSafeguards).
const safeguards = `"safeguards":[{"type":"dangerous_tool_use","classifier_context":"The user allows file edits in the project."}],`
