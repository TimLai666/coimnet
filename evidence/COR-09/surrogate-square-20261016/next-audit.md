# Next read-only hypothesis

Status: NEEDS INVESTIGATION. This note does not establish a new contract defect or change ticket 115's source.

The read-only worker identified a separate theta_raw tail question. Ticket 11 decision 3 requires the threshold chain to multiply by span*sigmoid'(raw), and decision 9 accepts every finite raw. The current LIF ordinary/constants/coupled and Mixed threshold setup computes logistic(raw), then s*(1-s). At raw around 40 or 50, the stored s may round to one before the analytic sigmoid slope is below float64 range. Root read the actual LIF mapping and logistic function and the original ticket decisions. No public reproduction was run for this hypothesis.

Candidate discriminating case: one LIF node, DT=log(2), tau_syn=1, theta bounds [0,2], reset=-1, fast_sigmoid scale=1; bias/log_tau/initial zero, theta_raw=40, input=[[1]], upstream=[[1]]. The proposed reference is -psi*2*exp(-raw)/(1+exp(-raw))^2, evaluated independently. Check the public ThetaRaw gradient and the declared floating-point derivative policy before classifying or changing behavior. Existing raw +/-1000 saturation tests and normal raw-zero span tests do not answer this tail question.

If the public check supports a defect, inventory all threshold-value/slope producers and their ordinary/recompute/coupled consumers, write failing references first, and retain normal-model and saved-state controls. Do not infer a real-task quality improvement from a small tail derivative alone. This is the next investigation, not an additional implementation or a claim that all remaining bugs are known.

A separate candidate can distinguish a merely tiny slope from a representable scaled coefficient: use theta bounds [0,1e308], theta_raw=40, initial=1e308, input=[[1e308]], upstream=[[1]], and the same one-node settings. Candidate voltage and rounded base threshold should both be 1e308, giving u=0 and psi=1. The analytic threshold coefficient would be about 4.248e290 while the stored-s formula gives zero. This is an arithmetic prediction, not a public runtime result; validate construction, all forward values and every reverse field before treating it as a confirmed defect.
