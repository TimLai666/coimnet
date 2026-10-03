"""Verify every frozen pair against ticket 34 and summarize without selecting trials."""
from pathlib import Path
import json, math, hashlib

HERE = Path(__file__).resolve().parent
PREVIOUS = HERE.parent / 'stimulus-memory-20261003'
def read(name): return json.loads(name.read_text())
def sha(name): return hashlib.sha256(name.read_bytes()).hexdigest()
report = read(HERE / 'report.json')
old = read(PREVIOUS / 'rollout-summary.json')
old_groups = {(g['seed'], g['control'], g['phase']): g for g in old['groups'] if g['strategy'] == 'model'}
inference = {(r['seed'], r['control']): r for r in read(PREVIOUS / 'inference-report.json')['results']}
traces = {(r['seed'], r['control'], r['phase'], r['trial_id']): r['go_trace_sha256'] for r in read(HERE / 'legacy-trace-hashes.json')}
assert len(report['groups']) == 18
expected_ids = set(report['split']['test_trial_ids'])
assert len(expected_ids) == 13
summary = []
pairs = 0
for g in report['groups']:
    key = (g['seed'], g['control'], g['phase'])
    assert {t['trial_id'] for t in g['trials']} == expected_ids and len(g['trials']) == 13
    original_group = old_groups[key]
    assert g['parameter_sha256'] == original_group['parameter_sha256']
    assert g['optimizer_sha256'] == original_group['optimizer_sha256']
    original_trials = {t['trial_id']: t for t in original_group['trials']}
    scored = sum(t['recorded']['samples'] for t in g['trials'])
    original_mse = sum(t['recorded']['original_mse_cm2'] * t['recorded']['samples'] for t in g['trials']) / scored
    erased_mse = sum(t['recorded']['erased_mse_cm2'] * t['recorded']['samples'] for t in g['trials']) / scored
    expected = inference[key[:2]][key[2]]['test']
    assert scored == expected['samples'] and math.isclose(original_mse, expected['mse'], rel_tol=0, abs_tol=1e-15)
    for t in g['trials']:
        previous = original_trials[t['trial_id']]
        a, b = t['rollout']['original'], t['rollout']['erased']
        assert all(a[k] == previous[k] for k in previous if k != 'trace_sha256')
        assert t['rollout']['original_trace_sha256'] == traces[key + (t['trial_id'],)]
        assert t['snapshot_frozen'] and 0 < t['relocation_index'] <= t['rollout_index'] < t['history_rows']
        assert t['original_non_stimulus_sha256'] == t['erased_non_stimulus_sha256']
        assert t['original_later_stimulus_sha256'] == t['erased_later_stimulus_sha256']
        assert a['steps'] == b['steps'] == 200
        if g['phase'] == 'before' or g['control'] == 'no_stimulus' or t['condition'] == 'non-rewarded':
            assert t['recorded']['changed_rows'] == 0 and t['recorded']['erased_minus_original_mse_cm2'] == 0
            assert t['rollout']['changed_decisions'] == 0 and t['rollout']['max_position_separation_cm'] == 0
            assert t['rollout']['original_trace_sha256'] == t['rollout']['erased_trace_sha256']
        pairs += 1
    summary.append({'seed':g['seed'], 'control':g['control'], 'phase':g['phase'], 'trials':13, 'recorded_samples':scored,
                    'original_mse_cm2':original_mse, 'erased_mse_cm2':erased_mse, 'erased_minus_original_mse_cm2':erased_mse-original_mse,
                    'changed_recorded_trials':sum(t['recorded']['changed_rows']>0 for t in g['trials']),
                    'above_resolution_recorded_trials':sum(t['recorded']['above_resolution_rows']>0 for t in g['trials']),
                    'max_recorded_delta_cm':max(t['recorded']['max_delta_cm'] for t in g['trials']),
                    'max_action_delta_cm':max(t['rollout']['max_action_delta_cm'] for t in g['trials']),
                    'max_position_separation_cm':max(t['rollout']['max_position_separation_cm'] for t in g['trials']),
                    'original_hits':sum(t['rollout']['original']['success'] for t in g['trials']),
                    'erased_hits':sum(t['rollout']['erased']['success'] for t in g['trials'])})
assert pairs == 234 and len(traces) == 234
assert all(g['original_hits'] == 0 and g['erased_hits'] == 0 for g in summary)
result = {'schema_version':'coimnet-counterfactual-summary/v1','groups':summary,'pairs':pairs,'excluded':0,
          'legacy_original_summaries_equal':True,'legacy_original_complete_trace_fingerprints_equal':True,
          'legacy_weighted_mse_equal_with_abs_tolerance':1e-15,'paired_input_invariants':True,
          'zero_controls_exact':True,'snapshot_parameters_and_saved_optimizer_frozen':True,
          'report_sha256':sha(HERE/'report.json'),
          'previous_rollout_summary_sha256':sha(PREVIOUS/'rollout-summary.json'),
          'previous_inference_report_sha256':sha(PREVIOUS/'inference-report.json'),
          'trace_encoding':'Go json.Marshal typed step array; legacy Python hashes independently normalized from the complete external report',
          'conclusion':'Trained delivered and shuffled models respond to past stimulus in eight rewarded trials per seed. Erasure MSE differences change sign across seeds. Every original and erased model group remains 0/13; no useful-memory or navigation-success claim.'}
print(json.dumps(result,indent=2))
