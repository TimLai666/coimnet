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
a token in HF_TOKEN or saved by `hf auth login`.
"""

import argparse
import errno
import faulthandler
import fnmatch
import hashlib
import http.client
import json
import os
import shutil
import signal
import subprocess
import sys
import tarfile
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

try:
    import fcntl
except ImportError:  # Windows
    fcntl = None

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parent.parent
HF = "https://huggingface.co"
# Plain-text formats shrink a lot under heavy zstd; parquet, images, audio and
# video are already compressed, so a fast level is enough for them.
TEXT_SUFFIXES = {".json", ".jsonl", ".txt", ".csv", ".tsv", ".md", ".npy"}
CHUNK = 8 << 20
# SMB mounts on macOS answer large writes, and reads of just-written files,
# with EAGAIN for a while; these errors are retried instead of failing a run.
TRANSIENT = {errno.EAGAIN, errno.EWOULDBLOCK, errno.EINTR, errno.ETIMEDOUT}


def log(msg):
    print(msg, file=sys.stderr, flush=True)


def human(n):
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if abs(n) < 1000 or unit == "TB":
            return f"{n:.1f} {unit}" if unit != "B" else f"{n} B"
        n /= 1000


def default_dest():
    # The archives can live outside the repo, e.g. in a corpus folder shared by several projects.
    env = os.environ.get("COIMNET_DATASETS")
    return Path(env).expanduser() if env else REPO_ROOT / "data" / "datasets"


def hf_token():
    """HF_TOKEN, else the token saved by `hf auth login`."""
    if os.environ.get("HF_TOKEN"):
        return os.environ["HF_TOKEN"]
    home = Path(os.environ.get("HF_HOME", Path.home() / ".cache" / "huggingface")).expanduser()
    try:
        return (home / "token").read_text().strip() or None
    except OSError:
        return None


def request(url, headers=None):
    hdrs = {"User-Agent": "coimnet-dataset-fetch/1"}
    token = hf_token() if url.startswith(HF) else None
    if token:
        hdrs["Authorization"] = "Bearer " + token
    hdrs.update(headers or {})
    return urllib.request.Request(url, headers=hdrs)


def retry(fn, *args, attempts=12):
    for attempt in range(attempts):
        try:
            return fn(*args)
        except OSError as e:
            if e.errno not in TRANSIENT or attempt == attempts - 1:
                raise
            wait = min(2 ** (attempt + 1), 60)
            log(f"    retry in {wait}s after: {e}")
            time.sleep(wait)


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
            # Quote the path: names such as "Day_#_2.mp4" would otherwise lose
            # everything after "#" as a URL fragment.
            "url": f"{HF}/datasets/{repo}/resolve/{sha}/{urllib.parse.quote(f['path'])}",
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
    # Connections to large hosts sometimes end early without an error, so a
    # short file is resumed with a Range request like any other failure.
    for attempt in range(20):
        have = part.stat().st_size if part.exists() else 0
        if have == f["bytes"]:
            break
        headers = {"Range": f"bytes={have}-"} if have else {}
        try:
            with urllib.request.urlopen(request(f["url"], headers), timeout=120) as resp:
                if have and resp.status != 206:
                    have = 0  # server ignored Range; start over
                with open(part, "ab" if have else "wb") as out:
                    shutil.copyfileobj(resp, out, CHUNK)
            err = f"connection closed at {human(part.stat().st_size)} of {human(f['bytes'])}"
        except urllib.error.HTTPError as e:
            if e.code in (401, 403):
                raise GatedError(f["url"]) from e
            if e.code == 416:  # already complete
                break
            err = e
        except (OSError, http.client.HTTPException) as e:
            err = e
        if part.exists() and part.stat().st_size == f["bytes"]:
            break
        wait = min(2 ** (attempt + 1), 60)
        log(f"    retry in {wait}s after: {err}")
        time.sleep(wait)
    else:
        raise RuntimeError(f"download failed: {f['url']}")
    got = part.stat().st_size
    if got != f["bytes"]:
        raise RuntimeError(f"{f['path']}: expected {f['bytes']} bytes, got {got}")
    part.rename(dest)


def file_hash(path, algo="sha256"):
    def once():
        h = hashlib.new(algo)
        with open(path, "rb") as fh:
            while chunk := fh.read(CHUNK):
                h.update(chunk)
        return h.hexdigest()
    return retry(once)


def copy_resumable(src, dst):
    """Copy src to dst, continuing from what dst already holds after each transient error."""
    size = src.stat().st_size

    def step():
        have = dst.stat().st_size if dst.exists() else 0
        if have > size:
            dst.unlink()
            have = 0
        with open(src, "rb") as fin, open(dst, "ab") as fout:
            fin.seek(have)
            shutil.copyfileobj(fin, fout, CHUNK)
    retry(step)


# ---------- packing ----------

def textify(path):
    """Rewrite a parquet file whose columns hold no binary data as JSON Lines.

    Parquet pages are already compressed (mostly snappy), which leaves little
    for zstd. The same rows as plain text compress about 2.8x smaller under
    zstd -19 (TinyStories: 249 MB parquet -> 89 MB). Every column and value is
    kept; only the container changes. The caller removes the parquet once it
    has recorded the hashes. Returns the new path, or None when the
    file has binary columns (images, audio) or pyarrow is not installed.
    """
    try:
        import pyarrow as pa
        import pyarrow.parquet as pq
    except ImportError:
        return None

    def has_binary(t):
        if pa.types.is_binary(t) or pa.types.is_large_binary(t) or pa.types.is_fixed_size_binary(t):
            return True
        if pa.types.is_struct(t):
            return any(has_binary(t.field(i).type) for i in range(t.num_fields))
        if pa.types.is_list(t) or pa.types.is_large_list(t) or pa.types.is_fixed_size_list(t):
            return has_binary(t.value_type)
        if pa.types.is_map(t):
            return has_binary(t.key_type) or has_binary(t.item_type)
        return False

    pf = pq.ParquetFile(path)
    if any(has_binary(f.type) for f in pf.schema_arrow):
        return None
    out = path.with_name(path.name + ".jsonl")
    tmp = out.with_name(out.name + ".part")
    with open(tmp, "w", encoding="utf-8") as fh:
        for batch in pf.iter_batches(batch_size=8192):
            for row in batch.to_pylist():
                fh.write(json.dumps(row, ensure_ascii=False, default=str) + "\n")
    tmp.rename(out)
    return out

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


def hold_lock(path):
    """Refuse to work on a dataset another fetch.py process is already working on.

    Two runs on the same work directory rename and delete each other's .part
    files. The lock is released when the returned file is closed or the process ends.
    """
    fh = open(path, "a")
    if fcntl:
        try:
            fcntl.flock(fh, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            fh.close()
            raise RuntimeError(f"another fetch.py is already working on this dataset ({path})")
    return fh


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
    lock = hold_lock(work / f".{e['id']}.lock")  # noqa: F841 (held until run returns)
    raw = work / e["id"]
    for i, f in enumerate(files, 1):
        # A parquet already converted by an interrupted run is gone; its sidecar
        # keeps the hashes so the file is not downloaded again.
        side = raw / (f["path"] + ".src.json")
        if not (args.raw_only or args.keep_parquet) and side.exists() and (raw / (f["path"] + ".jsonl")).exists():
            f.update(json.loads(side.read_text()))
            log(f"  [{i}/{len(files)}] {f['path']} (converted earlier)")
            continue
        log(f"  [{i}/{len(files)}] {f['path']} ({human(f['bytes'])})")
        download(f, raw / f["path"])
    for f in files:
        if "stored_as" in f:
            if f.get("upstream_sha256") and f["upstream_sha256"] != f["sha256"]:
                raise RuntimeError(f"{f['path']}: sha256 differs from upstream")
            continue
        p = raw / f["path"]
        f["sha256"] = file_hash(p)
        if f.get("upstream_sha256") and f["upstream_sha256"] != f["sha256"]:
            raise RuntimeError(f"{f['path']}: sha256 differs from upstream")
        if f.get("upstream_md5") and file_hash(p, "md5") != f["upstream_md5"]:
            raise RuntimeError(f"{f['path']}: md5 differs from upstream")
    if args.raw_only:
        log(f"  verified {len(files)} files in {raw}")
        return "downloaded"
    for f in files:
        if "stored_as" in f:
            continue
        f["stored_as"] = f["path"]
        if f["path"].endswith(".parquet") and not args.keep_parquet:
            src = raw / f["path"]
            out = textify(src)
            if out:
                f["stored_as"] = out.relative_to(raw).as_posix()
                f["stored_sha256"] = file_hash(out)
                side = raw / (f["path"] + ".src.json")
                side.write_text(json.dumps({k: f[k] for k in ("sha256", "stored_as", "stored_sha256")}))
                src.unlink()
    level = 19 if all(Path(f["stored_as"]).suffix in TEXT_SUFFIXES for f in files) else 3
    log(f"  packing into {archive.name} (zstd -{level}) ...")
    local = work / archive.name
    pack(raw, [f["stored_as"] for f in files], local, level)
    archive_bytes, archive_sha256 = local.stat().st_size, file_hash(local)
    if local != archive:
        # Pack on local disk, then copy once: the destination may be a network mount.
        part = archive.with_name(archive.name + ".part")
        copy_resumable(local, part)
        if file_hash(part) != archive_sha256:
            part.unlink()
            raise RuntimeError(f"copy to {archive} differs from the packed archive")
        retry(part.rename, archive)
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
        "files": [{k: f[k] for k in ("path", "bytes", "sha256", "url", "stored_as", "stored_sha256") if k in f}
                  for f in files],
        "raw_bytes": total,
        "archive": archive.name,
        "archive_bytes": archive_bytes,
        "archive_sha256": archive_sha256,
    }
    retry(receipt.write_text, json.dumps(rec, indent=2, ensure_ascii=False) + "\n")
    log(f"  done: {human(total)} -> {human(archive_bytes)}")
    if not args.keep_raw:
        shutil.rmtree(raw)
    return "packed"


def on_signal(signum, frame):
    log(f"! stopped by signal {signal.Signals(signum).name}")
    sys.exit(128 + signum)


def main():
    # Two runs ended with no message at all; a native crash now prints a
    # traceback and a termination signal leaves a line in the log.
    faulthandler.enable()
    for sig in (signal.SIGTERM, signal.SIGHUP):
        if signal.getsignal(sig) != signal.SIG_IGN:  # keep nohup's ignored SIGHUP
            signal.signal(sig, on_signal)
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--catalog", type=Path, default=HERE / "catalog.json")
    ap.add_argument("--dest", type=Path, default=default_dest(),
                    help="storage root (default: $COIMNET_DATASETS, else <repo>/data/datasets)")
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
    ap.add_argument("--keep-parquet", action="store_true",
                    help="store text parquet files as-is instead of converting them to JSON Lines")
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

    if not (args.keep_parquet or args.raw_only):
        try:
            import pyarrow  # noqa: F401
        except ImportError:
            # Packing parquet as-is would silently make text archives about 2.8x larger.
            log("! pyarrow is not installed for this python; text parquet would be packed unconverted.\n"
                "  Install it (python3 -m pip install pyarrow) or pass --keep-parquet.")
            return 2

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
