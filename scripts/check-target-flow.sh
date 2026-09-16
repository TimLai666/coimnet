#!/usr/bin/env bash
set -euo pipefail

# SIG-03, the compile-time half of the separation proof.
#
# A training target may reach exactly one place: the loss function, which the
# trainer feeds from a plain []float64 at learning.Trainer.Step. This script
# proves the other half of that sentence, that no forward or modulation layer
# can name a target at all:
#
#   1. dynamics, simulate, modulation and plasticity never mention signal.Target
#      or signal.NewTarget in a non-test file, and neither does learning, whose
#      targets arrive as []float64 and never as the signal type;
#   2. none of those four packages depends on learning, so none of them can
#      reach a trainer, an episode or a loss by any import path.
#
# Both checks are read-only. A violation prints the offending lines and exits 1.
# Run it from anywhere; it resolves the repository root itself.

usage() {
  echo 'Usage: scripts/check-target-flow.sh'
  echo 'Checks that no forward or modulation package names signal.Target or imports learning (SIG-03).'
  echo 'Takes no argument. Prints the dependency list it checked, then PASS or the offending lines.'
  echo 'Exit status: 0 pass, 1 violation, 2 usage or missing package.'
}

if [[ $# != 0 ]]; then
  if [[ ${1} == --help || ${1} == -h ]]; then usage; exit 0; fi
  usage
  exit 2
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
module=github.com/TimLai666/coimnet

# The forward and modulation layers. They must neither name a target nor be able
# to import the package that owns one.
blind=(dynamics simulate modulation plasticity)
# learning owns the loss, so it may hold a target; it may not hold the type.
target_free=("${blind[@]}" learning)

status=0
echo "repository: $root"
echo "go: $(go version)"
echo

for package in "${target_free[@]}"; do
  if [[ ! -d $package ]]; then
    echo "SKIP $package: the package does not exist yet"
    continue
  fi
  hits=$(grep -rn 'signal\.Target\|NewTarget(' --include='*.go' "$package" | grep -v '_test\.go:' || true)
  if [[ -n $hits ]]; then
    echo "FAIL $package names a training target in a non-test file:"
    printf '%s\n' "$hits"
    status=1
  else
    files=$(find "$package" -type f -name '*.go' ! -name '*_test.go' | wc -l | tr -d ' ')
    echo "PASS $package: no signal.Target and no NewTarget( in $files non-test Go files"
  fi
done
echo

for package in "${blind[@]}"; do
  if [[ ! -d $package ]]; then
    echo "SKIP $package: the package does not exist yet"
    continue
  fi
  if ! deps=$(go list -deps -f '{{.ImportPath}}' "./$package" 2>&1); then
    echo "FAIL $package: go list -deps failed:"
    printf '%s\n' "$deps"
    status=1
    continue
  fi
  if printf '%s\n' "$deps" | grep -qx "$module/learning"; then
    echo "FAIL $package depends on $module/learning:"
    printf '%s\n' "$deps" | grep "^$module/"
    status=1
    continue
  fi
  echo "PASS $package does not depend on $module/learning. Its $module dependencies are:"
  printf '%s\n' "$deps" | grep "^$module/" | sed 's/^/  /'
done
echo

if [[ $status == 0 ]]; then
  echo 'RESULT: a target reaches no forward or modulation package (SIG-03 compile-time check passed)'
else
  echo 'RESULT: target flow violation'
fi
exit $status
