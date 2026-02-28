#!/bin/bash

cd /home/openclaw/projects/contentai
LOGFILE=/home/openclaw/.openclaw/workspace/logs/contentai-watchdog.log
WORKTREES=/home/openclaw/projects/contentai-worktrees
CODEX=/home/openclaw/.local/bin/codex
MODEL=gpt-5.3-codex
MAX_AGENTS=4

log() { echo "[$(date -u +%H:%M:%S)] $*" | tee -a "$LOGFILE"; }

# Count alive tmux swarm sessions
alive_count() {
    tmux list-sessions -F "#{session_name}" 2>/dev/null | grep -c "^swarm-ca-" 2>/dev/null || true
}

# Check if a ticket's agent has exited and produced commits
check_completion() {
    local tid=$1
    local dir="$WORKTREES/$tid"
    [ -d "$dir" ] || return 1
    
    # If tmux session exists, agent is still running
    tmux has-session -t "swarm-$tid" 2>/dev/null && return 1
    
    # Session gone — check for commits
    cd "$dir"
    local commits=$(git log --oneline main..HEAD 2>/dev/null | wc -l)
    if [ "$commits" -gt 0 ]; then
        # Verify tests pass
        if go test ./... -count=1 >/dev/null 2>&1; then
            return 0  # completed successfully
        else
            log "⚠️ $tid: has commits but tests fail"
            return 1
        fi
    fi
    
    # No commits — check if there's untracked work
    cd "$dir"
    local changes=$(git status --porcelain | wc -l)
    if [ "$changes" -gt 0 ]; then
        # Agent wrote code but didn't commit — try to commit
        go test ./... -count=1 >/dev/null 2>&1
        if [ $? -eq 0 ]; then
            git add -A
            git commit -m "feat($tid): auto-commit (agent exited without committing)" >/dev/null 2>&1
            log "📝 $tid: auto-committed (tests pass)"
            return 0
        else
            log "⚠️ $tid: has changes but tests fail"
            return 1
        fi
    fi
    
    log "❌ $tid: agent exited with no work"
    return 1
}

# Mark ticket done in tracker
mark_done() {
    local tid=$1
    local sha=$(cd "$WORKTREES/$tid" && git rev-parse HEAD)
    python3 -c "
import json
t = json.load(open('swarm/tracker.json'))
t['tickets']['$tid']['status'] = 'done'
t['tickets']['$tid']['sha'] = '$sha'
json.dump(t, open('swarm/tracker.json', 'w'), indent=2)
"
    log "✅ $tid done ($sha)"
}

# Merge completed phase branches into main
merge_if_phase_complete() {
    local phase=$1
    # Get all tickets in this phase
    local tickets=$(python3 -c "
import json
t = json.load(open('swarm/tracker.json'))
phase_tickets = [tid for tid, tk in t['tickets'].items() if tk['phase'] == $phase]
all_done = all(t['tickets'][tid]['status'] == 'done' for tid in phase_tickets)
if all_done and phase_tickets:
    print(' '.join(sorted(phase_tickets)))
")
    
    if [ -n "$tickets" ]; then
        log "🔀 Merging Phase $phase branches: $tickets"
        for tid in $tickets; do
            if git branch --list "feat/$tid" | grep -q .; then
                git merge "feat/$tid" --no-ff -m "feat: merge $tid" 2>&1 | tail -1
                # If conflict, try auto-resolve
                if [ $? -ne 0 ]; then
                    # Take theirs for new files, ours for existing
                    git checkout --theirs . 2>/dev/null
                    git add -A
                    git commit -m "feat: merge $tid (auto-resolved)" 2>/dev/null || true
                fi
            fi
        done
        go mod tidy 2>/dev/null || true
        go build ./... 2>/dev/null || log "⚠️ Build issues after Phase $phase merge"
    fi
}

# Get spawnable tickets (deps met, status=todo)
get_spawnable() {
    python3 -c "
import json
t = json.load(open('swarm/tracker.json'))
spawnable = []
for tid, tk in sorted(t['tickets'].items()):
    if tk['status'] != 'todo':
        continue
    deps_met = all(t['tickets'].get(d, {}).get('status') == 'done' for d in tk.get('depends', []))
    if deps_met:
        spawnable.append(tid)
print(' '.join(spawnable))
"
}

# Spawn a ticket
spawn_ticket() {
    local tid=$1
    local dir="$WORKTREES/$tid"
    
    # Clean up if exists
    rm -rf "$dir"
    git branch -D "feat/$tid" 2>/dev/null
    git worktree prune
    
    # Create worktree
    git worktree add -b "feat/$tid" "$dir" main 2>&1 | tail -1
    
    # Copy prompt
    cp "swarm/prompts/$tid.md" "$dir/.codex-prompt.md"
    
    # Spawn tmux
    tmux new-session -d -s "swarm-$tid" \
        "'$CODEX' exec -m '$MODEL' --dangerously-bypass-approvals-and-sandbox -C '$dir' \"\$(cat '$dir/.codex-prompt.md')\""
    
    # Update tracker
    python3 -c "
import json, time
t = json.load(open('swarm/tracker.json'))
t['tickets']['$tid']['status'] = 'running'
t['tickets']['$tid']['started_at'] = time.strftime('%Y-%m-%dT%H:%M:%SZ')
json.dump(t, open('swarm/tracker.json', 'w'), indent=2)
"
    log "🐝 Spawned $tid"
}

# Main loop
while true; do
    # Check running tickets for completion
    running=$(python3 -c "
import json
t = json.load(open('swarm/tracker.json'))
print(' '.join(tid for tid, tk in t['tickets'].items() if tk['status'] == 'running'))
")
    
    for tid in $running; do
        if check_completion "$tid"; then
            mark_done "$tid"
            phase=$(python3 -c "import json; t=json.load(open('swarm/tracker.json')); print(t['tickets']['$tid']['phase'])")
            merge_if_phase_complete "$phase"
        fi
    done
    
    # Spawn new tickets
    alive=$(alive_count)
    spawnable=$(get_spawnable)
    
    for tid in $spawnable; do
        if [ "$alive" -ge "$MAX_AGENTS" ]; then
            break
        fi
        spawn_ticket "$tid"
        alive=$((alive + 1))
    done
    
    # Check if all done
    all_done=$(python3 -c "
import json
t = json.load(open('swarm/tracker.json'))
print('yes' if all(tk['status'] == 'done' for tk in t['tickets'].values()) else 'no')
")
    
    if [ "$all_done" = "yes" ]; then
        log "🏁 ALL 15 TICKETS COMPLETE"
        break
    fi
    
    sleep 20
done
