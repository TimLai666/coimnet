#!/usr/bin/env bash
set -euo pipefail

# GOV-05 (ticket 30, stage 1): license inventory for release preparation.
#
# Reads the full module graph (direct and indirect) with go list -m -json all,
# constructs each module's cache directory from $(go env GOMODCACHE) + the
# case-encoded module path + version, downloads the module first if it is not
# in the cache, finds the first LICENSE/LICENCE/COPYING file, and classifies
# the license from the first 400 bytes with fixed heuristics.
#
# Writes the inventory to docs/licenses.md (overwriting it) and prints one
# AUDIT line per module plus a RESULT line. Only docs/licenses.md is written.
# Run it from the repository root.

usage() {
  echo 'Usage: scripts/license-inventory.sh'
  echo 'Regenerates docs/licenses.md from the Go module graph (GOV-05).'
  echo 'Takes no argument. Downloads modules missing from the module cache.'
  echo 'Prints per-module AUDIT lines and a final RESULT line.'
  echo 'Exit status: 0 success, 1 failure, 2 usage.'
}

if [[ $# != 0 ]]; then
  if [[ ${1} == --help || ${1} == -h ]]; then usage; exit 0; fi
  usage
  exit 2
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
out="$root/docs/licenses.md"
modcache=$(go env GOMODCACHE)

# Encode a module path for the Go module cache: each uppercase letter in a path
# element becomes "!" followed by the lowercase letter.
encode_path() {
  local p=$1 result="" c i
  for ((i = 0; i < ${#p}; i++)); do
    c=${p:i:1}
    if [[ $c == [A-Z] ]]; then
      result+="!$(printf '%s' "$c" | tr 'A-Z' 'a-z')"
    else
      result+=$c
    fi
  done
  printf '%s' "$result"
}

# First LICENSE/LICENCE/COPYING file (case-insensitive) in a module directory.
license_file() {
  local d=$1
  find "$d" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'LICENCE*' -o -iname 'COPYING*' \) -print 2>/dev/null |
    sort | head -n 1
}

# Classify the license from the first 400 bytes. Unrecognized -> 未知.
classify() {
  local f=$1 text
  text=$(head -c 400 "$f")
  if grep -q 'MIT License' <<<"$text"; then echo MIT; return; fi
  if grep -q 'Apache License' <<<"$text" && grep -q '2\.0' <<<"$text"; then echo Apache-2.0; return; fi
  if grep -q 'BSD' <<<"$text" && grep -q 'Redistribution and use' <<<"$text"; then
    if grep -q 'Neither the name' <<<"$text"; then echo BSD-3-Clause; else echo BSD-2-Clause; fi
    return
  fi
  if grep -q 'Mozilla Public License' <<<"$text" && grep -q '2\.0' <<<"$text"; then echo MPL-2.0; return; fi
  if grep -q 'ISC' <<<"$text"; then echo ISC; return; fi
  echo 未知
}

mods=()
unknown_list=()
n=0
uk=0

# Parse go list -m -json all. Each module is a JSON object, but consecutive
# objects are not blank-line separated, so accumulate lines and act on the
# closing brace. Extract the fields with grep+cut ("\t\"Path\": \"value\"").
rec=""
process_record() {
  local path ver col dir lic licfile rellib
  path=$(grep -m1 '"Path": "' <<<"$rec" | cut -d'"' -f4 || true)
  ver=$(grep -m1 '"Version": "' <<<"$rec" | cut -d'"' -f4 || true)
  dir=$(grep -m1 '"Dir": "' <<<"$rec" | cut -d'"' -f4 || true)
  if grep -q '"Main": true' <<<"$rec"; then return; fi
  if [[ $ver == "" ]]; then return; fi
  if grep -q '"Indirect": true' <<<"$rec"; then col=間接; else col=直接; fi

  enc=$(encode_path "$path")
  moddir="$modcache/$enc@$ver"
  if [[ ! -d $moddir && -n $dir ]]; then moddir=$dir; fi

  licfile=""
  if [[ ! -d $moddir ]]; then
    if go mod download "${path}@${ver}" >/dev/null 2>&1; then
      if [[ ! -d $moddir && -n $dir ]]; then moddir=$dir; fi
    fi
  fi

  lic=""
  if [[ -d $moddir ]]; then
    licfile=$(license_file "$moddir")
  fi

  if [[ -n $licfile ]]; then
    lic=$(classify "$licfile")
    rellib="${moddir#$modcache/}/$(basename "$licfile")"
  elif [[ ! -d $moddir ]]; then
    lic=未下載
    rellib=""
  else
    lic=未知
    rellib=""
  fi

  mods+=("| ${path} | ${ver} | ${col} | ${lic} | ${rellib} |")
  n=$((n + 1))
  if [[ $lic == 未知 || $lic == 未下載 ]]; then
    unknown_list+=("${path}@${ver}: ${lic}")
    uk=$((uk + 1))
  fi
  printf 'AUDIT %-4s %-50s %-32s %s\n' "$col" "$path" "$ver" "$lic"
}

while IFS= read -r line; do
  if [[ $line == '{' ]]; then
    rec=""
  elif [[ $line == '}' ]]; then
    process_record
  else
    rec+="$line"$'\n'
  fi
done < <(go list -m -json all)

{
  echo '# 相依授權盤點'
  echo
  echo "產生時間（UTC）：$(date -u +'%Y-%m-%d %H:%M')"
  echo "go version：$(go version)"
  echo "模組總數：${n}，未知（含未下載）：${uk}"
  echo
  echo '| 模組 | 版本 | 直接／間接 | 授權 | 授權檔 |'
  echo '|---|---|---|---|---|'
  printf '%s\n' "${mods[@]}"
  echo
  echo '## 資料與素材'
  echo
  echo '- MaleCNS 發布資料：見 docs/malecns-source-audit.md'
  echo '- FlyWire：尚未匯入；取得時記錄條款與日期'
  echo '- 字型與媒體素材：無'
  echo
  echo '## 移植或改寫的程式碼'
  echo
  echo '無（若有須列出處與原授權）'
} > "$out"

if ((uk > 0)); then
  printf '\nUNKNOWN MODULES\n'
  printf '%s\n' "${unknown_list[@]}"
fi
echo "RESULT: modules=$n unknown=$uk"
exit 0