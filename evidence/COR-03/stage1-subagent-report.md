# Stage-one subagent report (Opus, 2026-09-14), verbatim excerpts kept as evidence

Red state before implementation (tests written first; first build failure, then stub assertions):

    # github.com/TimLai666/coimnet/dynamics [github.com/TimLai666/coimnet/dynamics.test]
    dynamics/lif_test.go:42:32: undefined: LIFConfig
    dynamics/lif_test.go:62:23: undefined: LIFParameters
    ...
    FAIL	github.com/TimLai666/coimnet/dynamics [build failed]

    --- FAIL: TestLIFHandCalculatedTiming/adaptation_off    lif_test.go:88: not implemented
    --- FAIL: TestLIFSurrogateGradientMatchesForwardModeReference  lif_test.go:316: not implemented
    --- FAIL: TestLIFSmoothModeFiniteDifference/adaptation_off_scale2  lif_test.go:440: not implemented
    --- FAIL: TestLIFConstructionRejectsInvalidConfig/theta_inverted   accepted theta inverted

Mutation checks reported by the subagent (implementation temporarily broken, then restored; each caught by a test):
smooth-mode reset gradient removed; theta_raw bounded-transform factor removed; window ignored;
smooth derivative missing 0.5*scale; refractory gradient leak; hard-mode reset passing gradient;
adaptation beta path removed; synaptic-trace kappa path removed.

Hand-calculation table (dt=ln2, tau=tau_syn=tau_adapt=1 so lambda=alpha=kappa=rho=0.5; edges 0->1 delay 0 w=2, 0->2 delay 2 w=4;
theta_raw=0 -> theta_base=1; v_reset=-1; refractory_steps=1; input to node 0: 4, 4, 3.2, 0, 4):

adaptation off
step | spike | v (after reset) | x | refractory
0 | 1,0,0 | -1,0,0 | 1,0,0 | 1,0,0
1 | 0,1,0 | -1,-1,0 | .5,1,0 | 0,1,0
2 | 1,0,0 | -1,-1,0 | 1.25,.5,0 | 1,0,0
3 | 0,0,1 | -1,.75,-1 | .625,.25,1 | 0,0,1
4 | 1,1,0 | -1,-1,-1 | 1.3125,1.125,.5 | 1,1,0

adaptation on (beta=0.5)
step | spike | v | x | a | refractory
0 | 1,0,0 | -1,0,0 | 1,0,0 | .5,0,0 | 1,0,0
1 | 0,1,0 | -1,-1,0 | .5,1,0 | .25,.5,0 | 0,1,0
2 | 0,0,0 | 1.1,-1,0 | .25,.5,0 | .125,.25,0 | 0,0,0
3 | 0,0,1 | .55,-.25,-1 | .125,.25,1 | .0625,.125,.5 | 0,0,1
4 | 1,0,0 | -1,0,-1 | 1.0625,.125,.5 | .53125,.0625,.25 | 1,0,0

Deviations accepted by root (see ticket 11): smooth test mode includes the reset dependency d v_next/d spike = v_reset - v_cand
so finite differences check the exact gradient of the smooth reference, while the product hard mode keeps the declared detached reset;
the smooth function's derivative is 0.5*scale*psi (equal to psi at scale 2), tests run scale 2 and 1.3; the surrogate reference test
uses an independent forward-mode (tangent) implementation of the declared rules with 1e-12 relative tolerance; tests are in-package
because the smooth mode is a private field. Only pre-call cancellation was exercised.
