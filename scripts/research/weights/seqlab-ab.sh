#!/bin/bash
# Weight-reading follow-up (2026-11-06): PyTorch seqlab A/B on column KMNIST, val protocol.
# ref = current softsign current-gate with clamp; dt = gate scales the integration step (no clamp);
# recip40 = random graph with 40% of edges in two-way pairs (real fly wiring has 39-44%).
cd /home/user/coimnet
OUT=${OUT:-runs/weight-reading-lab}; mkdir -p $OUT
D=${KMNIST_DIR:-data/TSK-11/kmnist}
exec 9>$OUT/run.lock; flock -n 9 || exit 0
job() { name=$1-s$2; spec=$3; [ -f $OUT/$name.result ] && return
  python3 scripts/research/seqlab.py --data $D --spec "$spec" --epochs 12 --lr 0.01 --seed $2 --threads 1 --val-count 10000 > $OUT/$name.log 2>&1 \
    && grep RESULT $OUT/$name.log > $OUT/$name.result; }
B='"n":512,"n_in":128,"n_out":512,"k":36,"act":"softsign"'
for s in 0 1 2; do
  job ref $s "{\"model\":\"coim\",\"topo\":\"random\",$B,\"gate\":\"local_softsign\"}" &
  job dt $s "{\"model\":\"coim\",\"topo\":\"random\",$B,\"gate\":\"dt_softsign\"}" &
  wait
  job recip40 $s "{\"model\":\"coim\",\"topo\":\"recip\",\"recip\":0.4,$B,\"gate\":\"local_softsign\"}"
done
echo done > $OUT/batch.done
