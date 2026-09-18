#!/usr/bin/env bash
set -euo pipefail

# STA-06 (ticket 24, stage 3): commit-time scan of the staged diff.
#
# Scans `git diff --cached` and blocks any commit containing:
#
#   1. secret            added lines with an API key or secret shape:
#                        sk-… seeds, Bearer-style headers, or an
#                        api_key/apikey/token/secret/password mapping to a value;
#   2. size              added or modified files larger than 5 MiB;
#   3. data              added files under data/ that are raw originals
#                        (data/raw/, data/malecns/, or any other data/** file that is not .md/.json);
#   4. teacher_response  added lines carrying teacher_response together with the
#                        "answer" field, i.e. raw teacher output.
#
# A hit prints one line per finding (file, line, rule) and exits 1. No hits
# prints PASS and exits 0. --help prints usage and exits 0. Anything other than
# a git repository prints the reason and exits 2.
#
# Read-only: it never writes a file, never touches the working tree and never
# modifies the index; it only reads the staged diff.
#
# The repository defaults to the current working directory. Override it with
# the first positional argument or with SCAN_COMMIT_ROOT.

usage() {
  echo 'Usage: scripts/scan-commit.sh [REPO]'
  echo 'Scans the staged diff for secrets, files > 5 MiB, raw data/ originals and teacher responses (STA-06).'
  echo 'REPO defaults to $SCAN_COMMIT_ROOT or the current directory.'
  echo 'Takes no flags apart from --help/-h. Read-only, changes nothing.'
  echo 'Exit status: 0 pass, 1 violation, 2 usage or not a git repository.'
}

if [[ $# -gt 1 ]]; then
  usage
  exit 2
fi
if [[ $# == 1 && ( ${1} == --help || ${1} == -h ) ]]; then
  usage
  exit 0
fi

repo=${1:-${SCAN_COMMIT_ROOT:-}}
if [[ -z $repo ]]; then
  repo=$(pwd)
fi

if ! git -C "$repo" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "scan-commit: not a git repository: $repo" >&2
  exit 2
fi

max_bytes=$((5 * 1024 * 1024))

pat_sk='sk-[A-Za-z0-9_-]{8,}'
bearer_head='Bearer'
pat_bearer="$bearer_head [^ ]+"
quote="[\"']"
pat_key_value="(api_key|apikey|token|secret|password)[[:space:]]*[=:][[:space:]]*${quote}?[A-Za-z0-9_-]{8,}"

violations=()

report_line() { # path line rule
  violations+=("$1:$2: rule=$3")
}

report_file() { # path rule
  violations+=("$1: rule=$2")
}

# Rule 1 and 4: added lines. Track "+++ b/<path>" per file and the new-side
# line number of the current diff hunk, then test every hunk line.
cur_file=""
newln=0
staged_diff=$(git -C "$repo" diff --cached -U0 --no-color 2>/dev/null) || {
  echo "scan-commit: cannot read the staged diff of $repo" >&2
  exit 2
}

while IFS= read -r line; do
  case "$line" in
    '+++ '*)
      cur_file=${line#'+++ b/'}
      if [[ $cur_file == /dev/null ]]; then cur_file=""; fi
      ;;
    '@@ '*)
      newside=${line#*+}
      newside=${newside%% @@*}
      if [[ $newside == *,* ]]; then
        newln=${newside%%,*}
      else
        newln=$newside
      fi
      ;;
    '+'*)
      if [[ -n $cur_file ]]; then
        content=${line#+}
        if grep -Eq "$pat_sk" <<<"$content" ||
          grep -Eq "$pat_bearer" <<<"$content" ||
          grep -qiE "$pat_key_value" <<<"$content"; then
          report_line "$cur_file" "$newln" secret
        fi
        ans='"answer"'
        if [[ $content == *teacher_response* && $content == *$ans* ]]; then
          report_line "$cur_file" "$newln" teacher_response
        fi
        newln=$((newln + 1))
      fi
      ;;
  esac
done <<<"$staged_diff"

# Rules 2 and 3: the staged file list, NUL-separated so names with spaces, tabs
# or newlines parse as a whole. A rename re-reads the destination path.
while IFS= read -r -d '' status; do
  IFS= read -r -d '' path || break
  if [[ $status == R* ]]; then
    IFS= read -r -d '' path || break
  fi
  if [[ $status == A ]]; then
    if [[ $path == data/raw/* || $path == data/malecns/* ||
      ( $path == data/* && $path != *.md && $path != *.json ) ]]; then
      report_file "$path" data
    fi
  fi
  case "$status" in
    A|M|R*)
      size=$(git -C "$repo" cat-file -s ":$path" 2>/dev/null || true)
      if [[ -n $size ]] && ((size > max_bytes)); then
        report_file "$path" size
      fi
      ;;
  esac
done < <(git -C "$repo" diff --cached -z --name-status)

if (( ${#violations[@]} > 0 )); then
  printf '%s\n' "${violations[@]}"
  echo "scan-commit: $repo: ${#violations[@]} violation(s) in the staged diff; commit blocked" >&2
  exit 1
fi
echo 'PASS'
echo "scan-commit: $repo: staged diff is clean"
exit 0