import hashlib, json
from pathlib import Path
root = Path('/Users/timlai/Developer/coimnet')
def sha(raw): return hashlib.sha256(raw).hexdigest()
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
    '[ticket 40](docs/tickets/40-six-step-deadline-training-comparison.md) 的期限': '[ticket 40](docs/tickets/40-six-step-deadline-training-comparison.md) 的期限對照已完成交付。方向學習只有一個 seed 通過，新的初始種子穩定性研究須採固定設定並另存證據。OPS-05 的裝置更新整合已完成，GPU 全圖與 Ubuntu RTX 4070 是未驗證的擴充範圍，不能由 Mac 小圖推論。',
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
