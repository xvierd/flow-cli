#!/bin/bash
# Claude Code status line integration for Flow
# Reads Claude's JSON from stdin, appends pomodoro status
# Install: add to ~/.claude/settings.json:
#   "statusLine": {
#     "type": "command",
#     "command": "~/.claude/flow-statusline.sh"
#   }

input=$(cat)

# Claude context info
MODEL=$(echo "$input" | jq -r '.model.display_name // "Claude"')
PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)

# Flow pomodoro status
FLOW_STATUS=""
if command -v flow &>/dev/null; then
  FLOW_JSON=$(flow status --json 2>/dev/null)
  if [ $? -eq 0 ] && [ -n "$FLOW_JSON" ]; then
    SESSION_STATUS=$(echo "$FLOW_JSON" | jq -r '.active_session.status // empty')
    if [ -n "$SESSION_STATUS" ]; then
      SESSION_TYPE=$(echo "$FLOW_JSON" | jq -r '.active_session.type')
      REMAINING=$(echo "$FLOW_JSON" | jq -r '.active_session.remaining_time')
      PROGRESS=$(echo "$FLOW_JSON" | jq -r '.active_session.progress')
      TASK=$(echo "$FLOW_JSON" | jq -r '.active_task.title // empty')

      # Parse remaining time (Go duration like "14m30s")
      MINS=$(echo "$REMAINING" | grep -oE '[0-9]+m' | tr -d 'm')
      SECS=$(echo "$REMAINING" | grep -oE '[0-9]+s' | tr -d 's')
      MINS=${MINS:-0}
      SECS=${SECS:-0}
      TIME_STR=$(printf "%02d:%02d" "$MINS" "$SECS")

      # Progress bar (5 chars wide)
      PCT_DONE=$(echo "$PROGRESS" | awk '{printf "%d", $1 * 5}')
      BAR=""
      for i in 1 2 3 4 5; do
        if [ "$i" -le "$PCT_DONE" ]; then
          BAR="${BAR}█"
        else
          BAR="${BAR}░"
        fi
      done

      # Color based on session type
      if [ "$SESSION_TYPE" = "work" ]; then
        ICON="🍅"
        COLOR="\033[31m"  # red
      else
        ICON="☕"
        COLOR="\033[32m"  # green
      fi
      RESET="\033[0m"

      if [ -n "$TASK" ]; then
        FLOW_STATUS="${COLOR}${ICON} ${TIME_STR} ${BAR} ${TASK}${RESET}"
      else
        FLOW_STATUS="${COLOR}${ICON} ${TIME_STR} ${BAR}${RESET}"
      fi
    fi
  fi
fi

# Build status line
if [ -n "$FLOW_STATUS" ]; then
  echo -e "[$MODEL] ${PCT}% ctx | $FLOW_STATUS"
else
  echo "[$MODEL] ${PCT}% ctx"
fi
