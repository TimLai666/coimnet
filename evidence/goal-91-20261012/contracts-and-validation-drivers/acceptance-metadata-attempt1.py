import copy, hashlib, json
from pathlib import Path

root = Path('/Users/timlai/Developer/coimnet')
goal = root / 'evidence/goal-91-20261012'
full = json.loads((goal / 'full-validation/complete/verification.json').read_text())
assert full['observed_result']['all_seven_checks_passed']
assert full['observed_result']['main_and_isolated_copy_match']

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def exclusive(path, obj):
    with path.open('x') as f:
        json.dump(obj, f, ensure_ascii=False, indent=2)
        f.write('\n')

def replace(path, old, new, expected=None):
    raw = path.read_bytes()
    if expected:
        assert sha(raw) == expected, path
    s = raw.decode()
    assert s.count(old) == 1, (path, old)
    result = s.replace(old, new)
    assert path.read_bytes() == raw, path
    path.write_text(result)

sources = json.loads((goal / 'full-validation/complete/source-frozen.json').read_text())
assert len(sources) == 708
for path, expected in sources.items():
    assert sha((root / path).read_bytes()) == expected, path

current = {
    'schema_version': 'coimnet-goal-acceptance/v1',
    'requirement': ['TSK-11', 'OPS-05'],
    'date': '2026-10-12',
    'profile': 'passed_engineering_acceptance_delivery_pending',
    'reproduction_command': ['See full-validation/complete/commands.json and environment.json for all seven integrated commands.', 'See each scoped verification for licensed-data training, actual Metal execution and new-process restore commands.'],
    'environment': full['environment'],
    'input_fingerprints': full['input_fingerprints'],
    'observed_result': {
        'requirements_passed': 91, 'requirements_total': 91,
        'engineering_acceptance_passed': True,
        'full_validation': full['observed_result'],
        'root_scoped_verification': 'evidence/goal-91-20261012/root-scoped-verification.json',
        'cleanup_complete': False,
        'delivery_complete': False,
        'quality': {'ocr_single_character_cer': 0.9, 'ocr_exact_ratio': 0.1, 'ocr_empty_output_ratio': 0, 'navigation_hits': '0/13', 'text': 'Held-out perplexity improves; generated text is fragmented.', 'media': 'Held-out balanced MSE and audio MSE worsen; generated frames are nearly black.', 'speech': 'No useful natural speech-recognition quality established.'},
    },
    'test_log': full['test_log'],
    'child_evidence': [
        'evidence/TSK-11/task-checkpoint-20261012/native-verification.json',
        'evidence/TSK-11/task-checkpoint-20261012/smb-verification.json',
        'evidence/TSK-11/ocr-raster-encoder-20261012/verification.json',
        'evidence/TSK-11/media-bundle-20261012/verification.json',
        'evidence/OPS-05/device-adam-20261012/verification.json',
        'evidence/OPS-05/device-training-20261012/verification.json',
        'evidence/goal-91-20261012/full-validation/complete/verification.json',
        'evidence/goal-91-20261012/doctor-verification.json',
    ],
    'acceptance_interpretation': {'sources': ['docs/handoff/requirements.json', 'docs/handoff/CoImNet_Implementation_Plan.zh-TW.md', 'ENG.md'], 'TSK-11': 'Five licensed real task classes import, train, independently infer and evaluate, with task model persistence and the section 13.2 image/page/encoder comparison interfaces. Section 20.2 permits negative research results.', 'OPS-05': 'Actual sparse device forward/backward, device AdamW update and restore, independent CPU comparison, phase and logical buffer measurements. Section 17.4 full residency is conditional on feasibility; OPS-07 separately records CPU full-graph execution.'},
    'limitations': ['91/91 is engineering acceptance of the original 85 requirements plus 6 NAT requirements. It does not establish a useful general model, learned biological navigation or a connectome advantage.', 'Actual device training is the declared scalar zero-delay float32 profile on Apple M3 Metal. Neural state, activations and sparse weight transfers remain on the host; Insyra encoder/readout device execution is not probed.', 'GPU full-graph capacity, speedup, allocator VRAM and Linux/Windows GPU execution remain unverified.', 'Scoped preservation receipts retain their creation-time pending fields and historical scratch paths. This parent and the final full-validation/cleanup receipts record subsequent closure without rewriting raw historical receipts.'],
}
exclusive(goal / 'verification.json', current)

archives = []
for rid, old_sha, archive_name in [
    ('TSK-11', '3e74f7530fbe8e081351d39c6e4c77b31468a7ed69a08dc16efe9e2c4813312b', 'verification-20260925-with-20261002-rollout.json'),
    ('OPS-05', 'af1794a126ba5ed660706fbb6db8fa27e8bc9d09af02609156208aade6d7880c', 'verification-20260925-before-device-training.json'),
]:
    path = root / 'evidence' / rid / 'verification.json'
    raw = path.read_bytes()
    assert sha(raw) == old_sha, rid
    archive = path.with_name(archive_name)
    with archive.open('xb') as f:
        f.write(raw)
    old = json.loads(raw)
    new = copy.deepcopy(old)
    new.update(date='2026-10-12', profile='passed_engineering_acceptance', environment=full['environment'])
    new['historical_evidence'] = {'path': str(archive.relative_to(root)), 'sha256': old_sha, 'note': 'Original metrics, environment, scope and limitations preserved byte for byte.'}
    new['reproduction_command'] = current['reproduction_command'] + ['See evidence/goal-91-20261012/verification.json for final integration and scoped evidence links.']
    new['test_log'] = old['test_log'] + full['test_log']
    new['input_fingerprints']['final_source_manifest'] = full['input_fingerprints']['source_manifest']
    new['input_fingerprints']['final_source_manifest_sha256'] = full['input_fingerprints']['source_manifest_sha256']
    new['input_fingerprints']['current_base_commit'] = full['input_fingerprints']['base_commit']
    new['final_acceptance'] = 'evidence/goal-91-20261012/verification.json'
    if rid == 'TSK-11':
        new['scope'] = 'Passed original engineering acceptance for five bounded licensed real-data task classes, task model persistence, PNG/JPEG decoding, same-model page OCR, fixed/trainable encoder comparison and empty-output reporting. Model-quality failures remain measured negative research results.'
        new['observed_result']['status'] = 'passed engineering acceptance; model usefulness and learned biological navigation are not established'
        new['observed_result']['current_completion'] = {'task_persistence_native_and_smb': 'evidence/TSK-11/task-checkpoint-20261012/', 'ocr_image_page_encoder': 'evidence/TSK-11/ocr-raster-encoder-20261012/', 'media_new_process_restore': 'evidence/TSK-11/media-bundle-20261012/', 'integration': 'evidence/goal-91-20261012/full-validation/complete/verification.json'}
        new['observed_result']['historical_metrics_note'] = 'Existing speech/text/trajectory metrics are historical runs. Current persistence, OCR paired modes and media restored outputs are separately dated and fingerprinted in current_completion; no historical run is presented as rerun.'
        new['limitations'] = [s for s in old['limitations'] if not s.startswith('The full race suite started')]
        new['limitations'] += current['limitations'][:1] + ['PNG/JPEG and same-recognizer page APIs are tested, but the real KMNIST quality sample contains only single characters. Fixed and trainable encoder modes both obtain CER 0.9, exact ratio 0.1 and empty-output ratio 0 on the declared 200-image test set.']
        new['child_evidence']['completion_20261012'] = {'verification': 'evidence/goal-91-20261012/verification.json', 'scope': current['acceptance_interpretation']['TSK-11']}
    else:
        new['scope'] = 'Passed original OPS-05 engineering acceptance: hardware sparse drive/local derivative, scalar zero-delay recurrent forward/backward, opt-in device AdamW update/atomic rejection, independent float32 CPU comparison, exact new-process training bundle continuation, phase and logical-buffer measurements.'
        new['observed_result'] = {'status': 'passed declared Apple M3 Metal profile', 'historical_primitives': old['observed_result']['ready'], 'device_adam': 'evidence/OPS-05/device-adam-20261012/verification.json', 'device_trainer': 'evidence/OPS-05/device-training-20261012/verification.json', 'new_process_bundle': 'evidence/OPS-05/device-training-20261012/bundle-run-final/verification.json', 'cpu_parameter_max_abs_difference': 2.798762427369539e-7, 'logical_buffer_bytes': 1548, 'declared_values': 23, 'integration': 'evidence/goal-91-20261012/full-validation/complete/verification.json'}
        new['assumption_statement'] = 'Insyra v0.3.4 remains the declared encoder/readout dependency. Its device execution was not probed. Device AdamW parameters/moments/counts are resident; neural state, activations and sparse weight transfers remain host managed. Timing is host wall-clock and buffer bytes are logical allocations.'
        new['limitations'] = current['limitations'][1:3] + old['limitations'][1:]
    assert path.read_bytes() == raw, path
    path.write_text(json.dumps(new, ensure_ascii=False, indent=2) + '\n')
    archives.append({'requirement': rid, 'archive': str(archive.relative_to(root)), 'sha256': old_sha})
exclusive(goal / 'historical-aggregator-preservation.json', {'archives': archives})

path = root / 'docs/requirements-status.json'
raw = path.read_bytes()
assert sha(raw) == 'a79295bf28989a572449c42752e5b927ba6478201773beca4ef7c36062b5d0da'
old = json.loads(raw)
new = copy.deepcopy(old)
assert len(new['requirements']) == 91
changed = []
for row in new['requirements']:
    if row['id'] in ('TSK-11', 'OPS-05'):
        assert row['status'] == 'specified', row
        row['status'] = 'passed'
        row['evidence'].append('../evidence/goal-91-20261012/verification.json')
        changed.append(row['id'])
assert sorted(changed) == ['OPS-05', 'TSK-11']
assert all(r['status'] == 'passed' for r in new['requirements'])
for before, after in zip(old['requirements'], new['requirements']):
    if before['id'] not in changed:
        assert before == after
assert path.read_bytes() == raw
path.write_text(json.dumps(new, ensure_ascii=False, indent=2) + '\n')

path = root / 'README.md'
raw = path.read_bytes()
assert sha(raw) == '32f5a234f2961793ada331625b6afec7f04f76e322dc999d1ba968a61e4500da'
s = raw.decode()
paragraphs = s.split('\n\n')
for prefix, text in [
    ('模型達標與完整 GPU 稀疏訓練仍未驗證', '原始 85 項與 NAT 六項工程需求已完成 91／91 驗收。公開授權資料已用於語音、文字、OCR、文字條件影音及果蠅軌跡範例，模型可保存後由新程序推論或接續。Apple M3 Metal 也已驗證稀疏前向、反向及裝置 AdamW 更新／恢復。真實任務的模型品質仍有限，GPU 全圖容量與加速尚未量測。各項範圍與負面結果見[開發進度](delivery-status.md)及[最終驗收](evidence/goal-91-20261012/verification.json)。'),
    ('TSK-11 已有語音、文字、單字元 OCR', 'TSK-11 的五類真實任務已完成匯入、訓練、獨立推論、評估與模型保存流程。PNG／JPEG 與同一模型的頁面辨識介面也已驗證，但真實 OCR 品質樣本限於 KMNIST 單字元。固定與可訓練編碼器都得到 CER 0.9、完整辨識比例 0.1，沒有證明可訓練模式較好。文字仍破碎，影音畫面接近全黑，語音、影音及原軌跡的保留集未改善。自主行走工程評估為 0／13，不支持真實導航或回巢能力。資料、授權及實測見[真實範例來源](docs/real-data-sources.md)與 [TSK-11 紀錄](evidence/TSK-11/verification.json)。OPS-05 已在 Apple M3 Metal 通過宣告的純量零延遲 profile：稀疏前向／反向、装置 AdamW 更新、完整訓練 bundle 新程序接續及量測。參數、動量與計數留在裝置，神經狀態與活化仍在主機。GPU 全圖容量、速度、實際顯示記憶體及 Linux／Windows GPU 尚未驗證，見 [OPS-05 紀錄](evidence/OPS-05/verification.json)。'.replace('装置', '裝置')),
]:
    hits = [i for i, p in enumerate(paragraphs) if p.startswith(prefix)]
    assert len(hits) == 1, prefix
    paragraphs[hits[0]] = text
s = '\n\n'.join(paragraphs)
anchor = '| 環境與自動驗證：'
lines = s.splitlines(keepends=True)
i = next(i for i, line in enumerate(lines) if line.startswith(anchor))
lines[i:i] = [
    '| 五類真實任務流程與模型保存：授權資料匯入、訓練、獨立推論／評估、PNG／JPEG、同模型頁面辨識、固定／可訓練編碼器對照（TSK-11；保留模型品質負面結果） | 已訓練（有限真實資料） | fixture（真實任務小樣本） | 核心能力與範例 | [TSK-11](evidence/TSK-11/verification.json) |\n',
    '| 可選 GPU 裝置訓練：稀疏前向／反向、float32 AdamW、候選原子提交、完整 bundle 新程序恢復與 CPU 對照（OPS-05；Apple M3 Metal，未量 GPU 全圖） | 已訓練（人工資料） | fixture | 核心能力 | [OPS-05](evidence/OPS-05/verification.json) |\n',
]
assert path.read_bytes() == raw
path.write_text(''.join(lines))

path = root / 'delivery-status.md'
raw = path.read_bytes()
assert sha(raw) == '9fc0c59dea9360a71957477fc9207f5d735ce64de7f238bd956048987e11312b'
s = raw.decode()
ps = s.split('\n\n')
replacements = {
    '目標是原始 91／91 驗收': '原始 85 項與 NAT 六項工程需求已完成 91／91 驗收，最終一般、race、格式、build、vet、相依性與治理檢查通過，708 份程式／模組指紋與主專案相同。tickets 59–63 的本機與 SMB 任務保存、實際 Metal 更新／新程序接續、OCR 圖片／頁面／編碼器對照、真實影音模型還原都有[最終證據](evidence/goal-91-20261012/verification.json)。模型品質負面結果與 GPU 全圖／其他平台未測範圍保留。交付與本輪暫存清理待完成。',
    '本階段處理 ticket 58': '本階段目標是完成原始 91／91 工程驗收。TSK-11 依 §13.1–13.6 完成五類有授權真實任務流程與保存恢復，OPS-05 完成裝置稀疏前向／反向、更新／恢復、CPU 對照與實測。§20.2 允許负面研究結果，§17.4 完整常駐是可行時的方向。模型實用性、GPU 全圖加速與生物機制成果分開判斷。'.replace('负面', '負面'),
    '目前專案由 mac-1': '目前沒有阻擋原始工程驗收的功能缺口。完整檢查在 TimdeAir 的原生 APFS 磁碟執行，SMB 只做小模型保存與新程序讀回。清理與提交仍待完成。NAS 斷線及伺服器故障後耐久性未實測。',
    'tickets 59、60 的完整一般測試通過': '本輪必要功能與最終完整驗證已完成，下一個可驗證成果是清理回條與已核對遠端的提交。目前沒有新的必要實作 ticket。',
    '[ticket 40](docs/tickets/40-six-step': '[ticket 40](docs/tickets/40-six-step-deadline-training-comparison.md) 的期限對照已完成交付。方向學習只有一個 seed 通過，新的初始種子穩定性研究須採固定設定並另存證據。OPS-05 的裝置更新整合已完成，GPU 全圖與 Ubuntu RTX 4070 是未驗證的擴充範圍，不能由 Mac 小圖推論。',
    '[共用設計](ENG.md)、[近期 tickets]': '[共用設計](ENG.md)、[近期 tickets](docs/tickets/)、[主規格](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md)、[91 項需求狀態（85 原始＋6 NAT）](docs/requirements-status.json)、[最終驗收](evidence/goal-91-20261012/verification.json)。交接原件與既有 LICENSE 保持不變。91／91 依原始工程條件驗收，模型品質、生物歸因及未測硬體須依各自證據判斷。',
}
for prefix, text in replacements.items():
    hits = [i for i, p in enumerate(ps) if p.startswith(prefix)]
    assert len(hits) == 1, prefix
    ps[hits[0]] = text
s = '\n\n'.join(ps)
s = s.replace('\n\n[ticket 58](docs/tickets/58-fullgraph-recompute-efficiency.md)', '\n\n### 較早階段交付紀錄\n\n以下狀態與數字保留各階段交付當時的範圍，最新累計以本頁首段及需求追蹤檔為準。\n\n[ticket 58](docs/tickets/58-fullgraph-recompute-efficiency.md)', 1)
s = s.replace('## 進行中\n\n', '## 里程碑\n\n| id | 目標 | 負責者 | 狀態 | 驗證訊號 |\n| --- | --- | --- | --- | --- |\n| 59 | OCR／語音／文字模型保存與新程序讀回 | Codex Root／Luna | in_progress | 功能與本機／SMB 比對通過，清理及交付待完成 |\n| 60 | 裝置 AdamW 更新與恢復 | Codex Root／Luna | in_progress | 實際 Metal 與 CPU 對照通過，清理及交付待完成 |\n| 61 | GPU 裝置訓練器及完整 bundle 接續 | Codex Root／Luna | in_progress | 部分累積窗的新程序精確接續通過，清理及交付待完成 |\n| 62 | 圖片／頁面辨識與編碼器對照 | Codex Root／Luna | in_progress | PNG／JPEG、頁面、真實 KMNIST 配對驗證通過，清理及交付待完成 |\n| 63 | 真實影音模型新程序生成 | Codex Root／Luna | in_progress | 120 次更新後 11 份模型／內容相同，清理及交付待完成 |\n\n### 較早階段紀錄\n\n以下是各階段當時的驗證狀態，保留歷史範圍及限制。\n\n', 1)
s = s.replace('\n\n前次直接在 SMB 執行', '\n\n### 既有環境與研究限制\n\n前次直接在 SMB 執行', 1)
assert path.read_bytes() == raw
path.write_text(s)

ticket_proofs = {
    '59-task-model-save-restore.md': 'evidence/TSK-11/task-checkpoint-20261012/native-verification.json、smb-verification.json',
    '60-device-adam-update-restore.md': 'evidence/OPS-05/device-adam-20261012/verification.json',
    '61-gpu-device-training.md': 'evidence/OPS-05/device-training-20261012/verification.json',
    '62-ocr-images-page-and-encoder.md': 'evidence/TSK-11/ocr-raster-encoder-20261012/verification.json',
    '63-realmedia-model-restore.md': 'evidence/TSK-11/media-bundle-20261012/verification.json',
}
ticket_hashes = {
    '59-task-model-save-restore.md': 'cf459aa7c8fb2d08e4998dd861599c95a4a83731d531847c61a3c2cdc5338d4e',
    '60-device-adam-update-restore.md': 'cb7edfba542fb95b990facc8cdddc9e4d045c121febbcab3278d407999d7a158',
    '61-gpu-device-training.md': '9ef9faa1b3320a9fa75aef398d1c749761b35596709da56b68c2b039ebc85cd0',
    '62-ocr-images-page-and-encoder.md': '6c5f4415a646c8b43dbcfea690537b3f36d5b58d731fe6fe4874cd674dd927b5',
    '63-realmedia-model-restore.md': '8675792d363ce073705c2ec7aef71eaec4a4c2fa93115d98814d908a901c43d1',
}
for filename, proof in ticket_proofs.items():
    path = root / 'docs/tickets' / filename
    raw = path.read_bytes()
    assert sha(raw) == ticket_hashes[filename], filename
    s = raw.decode()
    status = next(line for line in s.splitlines() if line.startswith('Status：'))
    s = s.replace(status, 'Status：功能與完整驗證通過，暫存清理及交付待完成。')
    boxes = [line for line in s.splitlines() if line.startswith('- [ ]')]
    assert boxes
    for line in boxes[:-1]:
        s = s.replace(line, line.replace('- [ ]', '- [x]', 1))
    for a, b in [('重開装置', '重開裝置'), ('恢复', '恢復'), ('递迴', '遞迴'), ('子程序实际', '子程序實際'), ('来源', '來源')]:
        s = s.replace(a, b)
    s += '\n## 驗收證據\n\n2026-10-12：' + proof + '。最終整合命令、日誌與來源指紋見 [91 項驗收](../../evidence/goal-91-20261012/verification.json)。清理完成後才勾選最後一項。\n'
    assert path.read_bytes() == raw
    path.write_text(s)
print('Engineering acceptance: 91/91. Delivery and cleanup remain pending; no executable source changed.')
