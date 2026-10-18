#!/usr/bin/env python3
"""Download the datasets in catalog.json and pack each one into a compressed archive.

Example only: CoImNet itself does not depend on these datasets.

Each dataset ends up as <dest>/archives/<id>.tar.zst plus a receipt
<dest>/archives/<id>.json that records the upstream revision, every file's
size and sha256, the license and the archive's sha256. Re-running skips
datasets whose archive and receipt already exist, and resumes partial
downloads. Uses only the Python standard library plus the `zstd` command
(falls back to .tar.gz when `zstd` is missing).

Gated Hugging Face datasets need the terms accepted on the dataset page and
a token in HF_TOKEN.
"""

import argparse
import fnmatch
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tarfile
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
HF = "https://huggingface.co"
# Plain-text formats shrink a lot under heavy zstd; parquet, images, audio and
# video are already compressed, so a fast level is enough for them.
TEXT_SUFFIXES = {".json", ".jsonl", ".txt", ".csv", ".tsv", ".md", ".npy"}
CHUNK = 8 << 20


def log(msg):
    print(msg, file=sys.stderr, flush=True)


def human(n):
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if abs(n) < 1000 or unit == "TB":
            return f"{n:.1f} {unit}" if unit != "B" else f"{n} B"
        n /= 1000


def request(url, headers=None):
    hdrs = {"User-Agent": "coimnet-dataset-fetch/1"}
    if url.startswith(HF) and os.environ.get("HF_TOKEN"):
        hdrs["Authorization"] = "Bearer " + os.environ["HF_TOKEN"]
    hdrs.update(headers or {})
    return urllib.request.Request(url, headers=hdrs)


def get_json(url):
    with urllib.request.urlopen(request(url), timeout=120) as resp:
        return json.load(resp), resp.headers


class GatedError(Exception):
    pass


# ---------- listing ----------

def list_hf(entry, tier):
    src = entry["source"]
    repo = src["repo"]
    try:
        info, _ = get_json(f"{HF}/api/datasets/{repo}/revision/main")
    except urllib.error.HTTPError as e:
        if e.code in (401, 403):
            raise GatedError(repo) from e
        raise
    sha = info["sha"]
    url = f"{HF}/api/datasets/{repo}/tree/{sha}?recursive=true"
    items = []
    while url:
        data, headers = get_json(url)
        items += data
        url = None
        link = headers.get("Link", "")
        if 'rel="next"' in link:
            url = link.split(";")[0].strip("<>")
    files = sorted(
        (i for i in items if i["type"] == "file"
         and any(fnmatch.fnmatch(i["path"], p) for p in src["patterns"])),
        key=lambda i: i["path"],
    )
    cap = (entry.get("max_files") or {}).get(tier)
    if cap is not None:
        files = files[:cap]
    out = []
    for f in files:
        size = f.get("lfs", {}).get("size", f["size"])
        out.append({
            "path": f["path"],
            "url": f"{HF}/datasets/{repo}/resolve/{sha}/{f['path']}",
            "bytes": size,
            "upstream_sha256": f.get("lfs", {}).get("oid"),
        })
    return {"type": "hf", "repo": repo, "revision": sha}, out


def list_url(entry, tier):
    files = [{"path": f["name"], "url": f["url"], "bytes": f["bytes"], "upstream_sha256": None}
             for f in entry["source"]["files"]]
    return {"type": "url"}, files


def list_zenodo(entry, tier):
    rec = entry["source"]["record"]
    data, _ = get_json(f"https://zenodo.org/api/records/{rec}")
    files = []
    for f in sorted(data["files"], key=lambda f: f["key"]):
        md5 = f["checksum"].split(":", 1)[1] if f.get("checksum", "").startswith("md5:") else None
        files.append({"path": f["key"], "url": f["links"]["self"], "bytes": f["size"],
                      "upstream_sha256": None, "upstream_md5": md5})
    return {"type": "zenodo", "record": rec, "version": data.get("metadata", {}).get("version")}, files


LISTERS = {"hf": list_hf, "url": list_url, "zenodo": list_zenodo}


# ---------- download ----------

def download(f, dest):
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() and dest.stat().st_size == f["bytes"]:
        return
    part = dest.with_name(dest.name + ".part")
    for attempt in range(5):
        have = part.stat().st_size if part.exists() else 0
        headers = {"Range": f"bytes={have}-"} if have else {}
        try:
            with urllib.request.urlopen(request(f["url"], headers), timeout=120) as resp:
                if have and resp.status != 206:
                    have = 0  # server ignored Range; start over
                with open(part, "ab" if have else "wb") as out:
                    shutil.copyfileobj(resp, out, CHUNK)
            break
        except urllib.error.HTTPError as e:
            if e.code in (401, 403):
                raise GatedError(f["url"]) from e
            if e.code == 416:  # already complete
                break
            err = e
        except (urllib.error.URLError, TimeoutError, ConnectionError) as e:
            err = e
        wait = 2 ** (attempt + 1)
        log(f"    retry in {wait}s after: {err}")
        time.sleep(wait)
    else:
        raise RuntimeError(f"download failed: {f['url']}")
    got = part.stat().st_size
    if got != f["bytes"]:
        raise RuntimeError(f"{f['path']}: expected {f['bytes']} bytes, got {got}")
    part.rename(dest)


def file_hash(path, algo="sha256"):
    h = hashlib.new(algo)
    with open(path, "rb") as fh:
        while chunk := fh.read(CHUNK):
            h.update(chunk)
    return h.hexdigest()


# ---------- packing ----------

def pack(raw_dir, paths, archive, level):
    """Write a deterministic tar (sorted names, zero mtime/owner) through zstd."""
    zstd = shutil.which("zstd")
    tmp = archive.with_name(archive.name + ".part")
    if zstd:
        proc = subprocess.Popen([zstd, "-q", "-T0", f"-{level}", "--long=27", "-f", "-o", str(tmp)],
                                stdin=subprocess.PIPE)
        sink, mode = proc.stdin, "w|"
    else:
        proc, sink, mode = None, open(tmp, "wb"), "w|gz"
    with tarfile.open(fileobj=sink, mode=mode, format=tarfile.PAX_FORMAT) as tar:
        for rel in sorted(paths):
            p = raw_dir / rel
            ti = tar.gettarinfo(str(p), arcname=rel)
            ti.mtime, ti.uid, ti.gid, ti.uname, ti.gname = 0, 0, 0, "", ""
            with open(p, "rb") as fh:
                tar.addfile(ti, fh)
    sink.close()
    if proc and proc.wait() != 0:
        raise RuntimeError("zstd failed")
    tmp.rename(archive)


# ---------- main ----------

def selected(catalog, args):
    order = {"core": 0, "full": 1}
    for e in catalog["datasets"]:
        if args.only and e["id"] not in args.only:
            continue
        if not args.only and order[e["tier"]] > order[args.tier]:
            continue
        if e["id"] in args.skip:
            continue
        if args.category and e["category"] not in args.category:
            continue
        if args.clear_only and e["license_status"] != "clear":
            continue
        yield e


def planned_bytes(e, tier):
    return e.get(f"{tier}_bytes") or (e.get("core_bytes") if tier == "core" else None) or e["bytes"]


def run(e, args, dest):
    ext = ".tar.zst" if shutil.which("zstd") else ".tar.gz"
    archive = dest / "archives" / f"{e['id']}{ext}"
    receipt = dest / "archives" / f"{e['id']}.json"
    if archive.exists() and receipt.exists() and not args.raw_only:
        log(f"= {e['id']}: already packed, skipping")
        return "skipped"
    pinned = args.match / f"{e['id']}.json" if args.match else None
    if pinned and pinned.exists():
        # Re-fetch exactly the files an earlier receipt recorded and require the same hashes.
        old = json.loads(pinned.read_text())
        source = old["source"]
        files = [{"path": f["path"], "url": f["url"], "bytes": f["bytes"], "upstream_sha256": f["sha256"]}
                 for f in old["files"]]
    else:
        source, files = LISTERS[e["source"]["type"]](e, args.tier)
    total = sum(f["bytes"] for f in files)
    log(f"+ {e['id']}: {len(files)} files, {human(total)}")
    work = args.work or dest / "raw"
    work.mkdir(parents=True, exist_ok=True)
    need = total * 2
    free = shutil.disk_usage(work).free
    if need > free:
        raise RuntimeError(f"needs about {human(need)} free in {work}, only {human(free)} left")
    raw = work / e["id"]
    for i, f in enumerate(files, 1):
        log(f"  [{i}/{len(files)}] {f['path']} ({human(f['bytes'])})")
        download(f, raw / f["path"])
    for f in files:
        p = raw / f["path"]
        f["sha256"] = file_hash(p)
        if f.get("upstream_sha256") and f["upstream_sha256"] != f["sha256"]:
            raise RuntimeError(f"{f['path']}: sha256 differs from upstream")
        if f.get("upstream_md5") and file_hash(p, "md5") != f["upstream_md5"]:
            raise RuntimeError(f"{f['path']}: md5 differs from upstream")
    if args.raw_only:
        log(f"  verified {len(files)} files in {raw}")
        return "downloaded"
    level = 19 if all(Path(f["path"]).suffix in TEXT_SUFFIXES for f in files) else 3
    log(f"  packing into {archive.name} ...")
    local = work / archive.name
    pack(raw, [f["path"] for f in files], local, level)
    archive_bytes, archive_sha256 = local.stat().st_size, file_hash(local)
    if local != archive:
        # Pack on local disk, then copy once: the destination may be a network mount.
        part = archive.with_name(archive.name + ".part")
        shutil.copyfile(local, part)
        if part.stat().st_size != archive_bytes:
            raise RuntimeError(f"copy to {archive} is incomplete")
        part.rename(archive)
        local.unlink()
    rec = {
        "schema_version": "coimnet-dataset-receipt/v1",
        "id": e["id"],
        "category": e["category"],
        "tier": args.tier,
        "license": e["license"],
        "license_status": e["license_status"],
        "source": source,
        "fetched_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "files": [{k: f[k] for k in ("path", "bytes", "sha256", "url")} for f in files],
        "raw_bytes": total,
        "archive": archive.name,
        "archive_bytes": archive_bytes,
        "archive_sha256": archive_sha256,
    }
    receipt.write_text(json.dumps(rec, indent=2, ensure_ascii=False) + "\n")
    log(f"  done: {human(total)} -> {human(archive_bytes)}")
    if not args.keep_raw:
        shutil.rmtree(raw)
    return "packed"


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--catalog", type=Path, default=HERE / "catalog.json")
    ap.add_argument("--dest", type=Path, default=REPO_ROOT / "data" / "datasets",
                    help="storage root (default: <repo>/data/datasets, ignored by git)")
    ap.add_argument("--work", type=Path, default=None,
                    help="local scratch for downloads and packing (default: <dest>/raw); "
                         "set it to local disk when --dest is a network mount")
    ap.add_argument("--tier", choices=["core", "full"], default="core",
                    help="core = small enough for one GPU box; full adds large and restricted sets")
    ap.add_argument("--only", nargs="*", default=[], help="dataset ids to fetch (ignores --tier)")
    ap.add_argument("--skip", nargs="*", default=[])
    ap.add_argument("--category", nargs="*", default=[])
    ap.add_argument("--clear-only", action="store_true", help="skip unclear and restricted licenses")
    ap.add_argument("--keep-raw", action="store_true", help="keep the unpacked download next to the archive")
    ap.add_argument("--match", type=Path, default=None,
                    help="directory of earlier receipts; datasets found there are re-fetched at the "
                         "recorded revision and must reproduce the recorded sha256 of every file")
    ap.add_argument("--raw-only", action="store_true",
                    help="download and verify into --work without packing (for training on a device)")
    ap.add_argument("--list", action="store_true", help="print the plan and exit")
    args = ap.parse_args()

    catalog = json.loads(args.catalog.read_text())
    entries = list(selected(catalog, args))
    if args.list:
        total = 0
        for e in entries:
            b = planned_bytes(e, args.tier)
            total += b
            flags = ("gated " if e["gated"] else "") + ("" if e["license_status"] == "clear" else e["license_status"])
            print(f"{e['id']:28} {e['category']:9} {human(b):>10}  {e['license']}  {flags}")
        print(f"{len(entries)} datasets, about {human(total)} before compression")
        return 0

    args.dest.mkdir(parents=True, exist_ok=True)
    (args.dest / "archives").mkdir(exist_ok=True)
    results = {}
    for e in entries:
        try:
            results[e["id"]] = run(e, args, args.dest)
        except GatedError:
            log(f"! {e['id']}: access denied. Accept the terms on the dataset page and set HF_TOKEN.")
            results[e["id"]] = "gated"
        except Exception as ex:  # keep going; report at the end
            log(f"! {e['id']}: {ex}")
            results[e["id"]] = f"failed: {ex}"
    log("\nsummary:")
    for k, v in results.items():
        log(f"  {k:28} {v}")
    return 0 if all(v in ("packed", "skipped", "downloaded") for v in results.values()) else 1


if __name__ == "__main__":
    sys.exit(main())
