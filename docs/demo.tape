# Demo recording for the README. Requires vhs (https://github.com/charmbracelet/vhs):
#   brew install vhs
#   go build -o flow ./cmd/flow && vhs docs/demo.tape
#
# Uses a throwaway HOME and an explicit --db seeded with a year of fake,
# deterministic history (scripts/demoseed), so your real ~/.flow is never
# touched. The seeder refuses to write to an existing file.

Output docs/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1000
Set Height 720
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms

Hide
Type `D=$(mktemp -d) && go run ./scripts/demoseed --db "$D/demo.db" >/dev/null && export HOME="$D" PATH="$PWD:$PATH" FLOW_NO_WATCHER=1 && alias flow='flow --db "$HOME/demo.db"' && cd "$D" && clear`
Enter
Wait
Sleep 500ms
Show

Type "flow wrapped --year"
Sleep 300ms
Enter
Sleep 5s

Hide
Type "clear"
Enter
Show

Type "flow stats --year"
Sleep 300ms
Enter
Sleep 5s

Hide
Type "clear"
Enter
Show

Type "flow badge -o focus.svg && flow wrapped --png wrapped.png"
Sleep 300ms
Enter
Sleep 5s
