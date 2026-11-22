# Demo recording for the README. Requires vhs (https://github.com/charmbracelet/vhs):
#   brew install vhs
#   go build -o flow ./cmd/flow && vhs docs/demo.tape
# Uses a throwaway HOME and database, so your real ~/.flow is never touched.

Output docs/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1100
Set Height 640
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms

Hide
Type `export HOME=$(mktemp -d) PATH="$PWD:$PATH" FLOW_NO_WATCHER=1 && alias flow='flow --db $HOME/demo.db' && clear`
Enter
Show

Type `flow add "Write API docs" -t docs,backend`
Enter
Sleep 1s
Type "flow list"
Enter
Sleep 2s

Type "flow"
Enter
Sleep 1500ms
Enter
Sleep 1s
Enter
Sleep 1s
Enter
Sleep 1s
Type "Write API docs #docs"
Sleep 500ms
Enter
Sleep 4s
Type "p"
Sleep 2s
Type "p"
Sleep 2s
Type "c"
Sleep 1s

Type "flow status"
Enter
Sleep 3s

Type "flow stop"
Enter
Sleep 500ms
Enter
Sleep 1s
Type "flow report --week"
Enter
Sleep 4s
