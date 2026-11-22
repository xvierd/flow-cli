package notification

import _ "embed"

// iconPNG is the Flow logo (a pulsar whose waves close into a clock dial),
// rendered from assets/flow.svg. Linux and Windows notifications show it
// directly; macOS uses the .icns in notify_darwin.go instead.
//
//go:embed assets/flow.png
var iconPNG []byte
