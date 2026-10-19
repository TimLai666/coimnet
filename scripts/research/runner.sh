#!/usr/bin/env bash
# runner.sh QUEUE_DIR runs queued research jobs one at a time, forever.
#
# A job is an executable file in QUEUE_DIR/pending. The runner moves it to
# running/, executes it with its output in logs/<job>.log, and then moves it to
# done/ or failed/. When pending/ is empty it polls every 30 seconds, so new
# jobs can be dropped in at any time. Jobs must be safe to restart: a job left
# in running/ by a crash or reboot goes back to pending/ when the runner starts.
#
# Start one runner per queue, detached, for example:
#   setsid nohup scripts/research/runner.sh "$W/queue/gpu" >/dev/null 2>&1 &
set -u
q=${1:?usage: runner.sh QUEUE_DIR}
mkdir -p "$q"/pending "$q"/running "$q"/done "$q"/failed "$q"/logs
exec 9>"$q/runner.lock"
if ! flock -n 9; then
	echo "runner.sh: another runner holds $q/runner.lock" >&2
	exit 1
fi
for job in "$q"/running/*; do
	[ -e "$job" ] && mv "$job" "$q/pending/"
done
while :; do
	job=$(ls "$q/pending" 2>/dev/null | sort | head -n 1)
	if [ -z "$job" ]; then
		sleep 30
		continue
	fi
	mv "$q/pending/$job" "$q/running/$job"
	echo "== $(date -Is) start $job" >>"$q/logs/runner.log"
	if bash "$q/running/$job" >>"$q/logs/$job.log" 2>&1; then
		mv "$q/running/$job" "$q/done/$job"
		echo "== $(date -Is) done $job" >>"$q/logs/runner.log"
	else
		code=$?
		mv "$q/running/$job" "$q/failed/$job"
		echo "== $(date -Is) failed $job ($code)" >>"$q/logs/runner.log"
	fi
done
