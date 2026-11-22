// Package scripts embeds the helper scripts shipped with Flow.
package scripts

import _ "embed"

// ClaudeStatusLine is the Claude Code status line script, installed by
// `flow claude install --statusline`.
//
//go:embed claude-statusline.sh
var ClaudeStatusLine []byte
