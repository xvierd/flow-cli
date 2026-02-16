#!/bin/bash
# Claude Code status line integration for Flow
# Reads Claude's JSON from stdin, appends pomodoro status
# Install: add to ~/.claude/settings.json:
#   "statusLine": {
#     "type": "command",
#     "command": "~/.claude/flow-statusline.sh"
#   }

input=$(cat)

# Claude context info — separate jq calls to handle spaces in model name
MODEL=$(echo "$input" | jq -r '.model.display_name // "Claude"' 2>/dev/null)
PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' 2>/dev/null)
PCT=${PCT%%.*}
MODEL=${MODEL:-Claude}
PCT=${PCT:-0}

# Flow pomodoro status
FLOW_STATUS=""
if command -v flow &>/dev/null; then
  FLOW_JSON=$(flow status --json 2>/dev/null) || FLOW_JSON=""
  if [ -n "$FLOW_JSON" ]; then
    # Extract fields with tab delimiter to handle spaces in task titles
    IFS=$'\t' read -r SESSION_STATUS SESSION_TYPE REMAINING PROGRESS TASK <<< "$(echo "$FLOW_JSON" | jq -r '
      [(.active_session.status // ""), (.active_session.type // ""), (.active_session.remaining_time // ""), (.active_session.progress // ""), (.active_task.title // "")] | join("\t")
    ' 2>/dev/null)"

    if [ -n "$SESSION_STATUS" ] && [ "$SESSION_STATUS" != "null" ]; then
      # Parse Go duration (e.g. "14m30.5s", "1h2m3s") using bash only
      TOTAL_SECS=0
      rem="$REMAINING"
      if [[ "$rem" =~ ([0-9]+)h ]]; then
        TOTAL_SECS=$(( TOTAL_SECS + ${BASH_REMATCH[1]} * 3600 ))
      fi
      if [[ "$rem" =~ ([0-9]+)m ]]; then
        TOTAL_SECS=$(( TOTAL_SECS + ${BASH_REMATCH[1]} * 60 ))
      fi
      if [[ "$rem" =~ ([0-9]+)(\.[0-9]+)?s ]]; then
        TOTAL_SECS=$(( TOTAL_SECS + ${BASH_REMATCH[1]} ))
      fi

      MINS=$((TOTAL_SECS / 60))
      SECS=$((TOTAL_SECS % 60))
      TIME_STR=$(printf "%02d:%02d" "$MINS" "$SECS")

      # Progress bar (5 chars wide)
      PCT_DONE=$(printf "%.0f" "$(echo "$PROGRESS * 5" | bc 2>/dev/null || echo 0)")
      BAR=""
      for i in 1 2 3 4 5; do
        if [ "$i" -le "${PCT_DONE:-0}" ]; then
          BAR="${BAR}█"
        else
          BAR="${BAR}░"
        fi
      done

      # Color based on session type
      YELLOW="\033[33m"
      RESET="\033[0m"
      if [ "$SESSION_TYPE" = "work" ]; then
        ICON="🍅"
        COLOR="\033[31m"
      else
        ICON="☕"
        COLOR="\033[32m"
      fi

      if [ -n "$TASK" ] && [ "$TASK" != "null" ]; then
        FLOW_STATUS="${COLOR}${ICON} ${TIME_STR} ${BAR} ${YELLOW}${TASK}${RESET}"
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
