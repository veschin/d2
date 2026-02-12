#!/usr/bin/env fish
# Collects [FORK] and [FORK:task] commits, groups by task, filters WIP/meta.
# Updates FORK_CHANGELOG.md between markers.

set changelog_file (dirname (status filename))/../FORK_CHANGELOG.md

if not test -f $changelog_file
    echo "FORK_CHANGELOG.md not found"
    exit 1
end

# Skip patterns — changelog updates, openspec docs, etc.
set -l skip_patterns \
    'Update FORK_CHANGELOG' \
    'Add OpenSpec change docs' \
    'deprecated.*to be removed'

# Collect all [FORK] commits (newest first).
set -l commits (git log --all --grep='\[FORK' --format='%h|%ad|%s' --date=short)

if test (count $commits) -eq 0
    echo "No [FORK] commits found"
    exit 0
end

echo "Found "(count $commits)" [FORK] commit(s)"

# Parse commits into task groups.
# Format: [FORK:task-name] description  or  [FORK] description (ungrouped)
set -l tasks
set -l task_entries

for commit in $commits
    set -l hash (string split '|' $commit)[1]
    set -l date (string split '|' $commit)[2]
    set -l subject (string split '|' $commit)[3]

    # Skip meta/WIP commits.
    set -l skip false
    for pat in $skip_patterns
        if string match -rq -- $pat $subject
            set skip true
            break
        end
    end
    if test $skip = true
        continue
    end

    # Extract task name: [FORK:task-name] or [FORK] (→ "other")
    set -l task other
    if string match -rq -- '^\[FORK:([^\]]+)\]' $subject
        set task (string match -r -- '^\[FORK:([^\]]+)\]' $subject)[2]
    end

    # Strip the [FORK...] prefix from description.
    set -l desc (string replace -r -- '^\[FORK[^\]]*\]\s*' '' $subject)

    # Track task order (first seen = newest commit).
    if not contains -- $task $tasks
        set -a tasks $task
    end

    # Store entry: task|date|hash|desc
    set -a task_entries "$task|$date|$hash|$desc"
end

# Build changelog content grouped by task.
set -l output ""

for task in $tasks
    # Collect entries for this task (already in newest-first order).
    set -l entries
    set -l first_date ""
    set -l last_date ""

    for entry in $task_entries
        set -l parts (string split '|' $entry)
        if test $parts[1] = $task
            set -a entries $parts[4]
            # Dates: first seen = newest, last seen = oldest.
            if test -z "$first_date"
                set first_date $parts[2]
            end
            set last_date $parts[2]
        end
    end

    # Format date range.
    set -l date_str $first_date
    if test "$first_date" != "$last_date"
        set date_str "$last_date → $first_date"
    end

    # Section header.
    set output "$output\n### $task ($date_str)\n"

    # List entries in chronological order (reverse of newest-first).
    for i in (seq (count $entries) -1 1)
        set output "$output\n- $entries[$i]"
    end
    set output "$output\n"
end

# Replace between markers.
set -l replacement "<!-- FORK_CHANGELOG:START -->$output\n<!-- FORK_CHANGELOG:END -->"

sed -i "/<!-- FORK_CHANGELOG:START -->/,/<!-- FORK_CHANGELOG:END -->/c\\$replacement" $changelog_file

echo "Updated $changelog_file"
