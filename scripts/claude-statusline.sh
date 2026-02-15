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

# Parse Go duration string to total seconds
# Handles: "1h2m3.456s", "14m30s", "45.5s", "0s"
parse_duration() {
  local dur="$1"
  local hours=0 mins=0 secs=0

  # Extract hours
  if echo "$dur" | grep -q 'h'; then
    hours=$(echo "$dur" | sed 's/h.*//' | grep -oE '[0-9]+$')
  fi
  # Extract minutes
  if echo "$dur" | grep -q 'm'; then
    mins=$(echo "$dur" | sed 's/.*h//' | sed 's/m.*//' | grep -oE '[0-9]+$')
  fi
  # Extract seconds (integer part only)
  if echo "$dur" | grep -q 's'; then
    secs=$(echo "$dur" | sed 's/.*m//' | sed 's/.*h//' | sed 's/s$//' | cut -d. -f1)
  fi

  hours=${hours:-0}
  mins=${mins:-0}
  secs=${secs:-0}

  echo $(( hours * 3600 + mins * 60 + secs ))
}

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

      TOTAL_SECS=$(parse_duration "$REMAINING")
      MINS=$((TOTAL_SECS / 60))
      SECS=$((TOTAL_SECS % 60))
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
