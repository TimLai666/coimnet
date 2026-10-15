# Root mechanism audit disposition

Three read-only workers and Root found no confirmed defect in the audited chemistry episode/checkpoint/update ownership. Existing tests already cover nonzero concentration, source windows, resources, delayed feedback, readout expression, LIF adaptation/homeostasis/refractory, accumulation and standalone new-process checkpoint continuation. These observations do not prove biological validity or every combination.

The four remaining evidence gaps are coverage candidates, not bugs: partial Filled plus multiple channels/expression under recomputation; Replay/InternalResource in learning-level recomputation; LIF chemical delay longer than segment; rich persistent Individual chemical boundaries with nondefault scheduling/clipping/truncation/accumulation in a new process. Repeated public gradient calls create independent traces and should not be misdescribed as repeating one private trace.

Root corrected a worker's wording: chemical ActivityGradient exclusion follows fixed truncation windows, not recompute segment boundaries. Source: learning/chemical_sequence.go BackwardStep, start=step/window*window, and global reverse ordering in dynamics/coupled_recompute_continuous.go.

During this audit Root followed the common gradient producer after the chemical bridge and publicly confirmed fixed-sign multiplication overflow. This is the active ticket118 defect; do not patch or reclassify speculative source-clock or cancellation observations. Trainer active gradients are already rejected by the norm before accumulation (learning/trainer.go:429-443). The direct public gradient interfaces still need a finite-result guard at their shared producer.
