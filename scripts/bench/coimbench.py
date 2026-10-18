"""Write coimnet-bench/v1 results from Python, matching the Go bench package.

A PyTorch (or any Python) model measured with these helpers produces the same
JSON as examples/benchocr, so `coimnet bench compare` can put the two side by
side. Host fields are read the same way the Go side reads them, which is what
lets the comparison confirm both ran on the same machine.
"""
import json
import math
import os
import platform
import resource
import socket
import subprocess
import sys
import time

SCHEMA_VERSION = "coimnet-bench/v1"


def _arch():
    m = platform.machine().lower()
    return {"x86_64": "amd64", "aarch64": "arm64"}.get(m, m)


def _os():
    return {"linux": "linux", "darwin": "darwin", "win32": "windows"}.get(sys.platform, sys.platform)


def cpu_model():
    if sys.platform == "linux":
        try:
            with open("/proc/cpuinfo") as f:
                for line in f:
                    name, _, value = line.partition(":")
                    if name.strip() == "model name":
                        return value.strip()
        except OSError:
            pass
    elif sys.platform == "darwin":
        try:
            return subprocess.check_output(["sysctl", "-n", "machdep.cpu.brand_string"], text=True).strip()
        except (OSError, subprocess.CalledProcessError):
            pass
    return _arch()


def total_memory():
    if sys.platform == "linux":
        try:
            with open("/proc/meminfo") as f:
                for line in f:
                    name, _, value = line.partition(":")
                    if name.strip() == "MemTotal":
                        return int(value.split()[0]) * 1024
        except (OSError, ValueError, IndexError):
            pass
    elif sys.platform == "darwin":
        try:
            return int(subprocess.check_output(["sysctl", "-n", "hw.memsize"], text=True).strip())
        except (OSError, ValueError, subprocess.CalledProcessError):
            pass
    return 0


def current_host(machine="", gpu="", runtime=""):
    return {
        "machine": machine or socket.gethostname(),
        "os": _os(),
        "arch": _arch(),
        "cpu": cpu_model(),
        "logical_cpus": os.cpu_count() or 0,
        "memory_bytes": total_memory(),
        "gpu": gpu,
        "runtime": runtime or "python" + platform.python_version(),
    }


def peak_rss():
    """Resident-set peak of this process so far, in bytes (Linux reports KiB)."""
    peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    return peak if sys.platform == "darwin" else peak * 1024


def _quantile(sorted_ms, q):
    # Nearest rank, the same rule as bench.quantile in Go.
    rank = max(math.ceil(q * len(sorted_ms)) - 1, 0)
    return sorted_ms[rank]


def measure(warmup, samples, fn):
    """Time fn(i) once per i in range(samples) after warmup untimed calls."""
    if warmup < 0 or samples <= 0:
        raise ValueError("need warmup >= 0 and samples > 0")
    for i in range(warmup):
        fn(i % samples)
    times = []
    for i in range(samples):
        start = time.perf_counter_ns()
        fn(i)
        times.append((time.perf_counter_ns() - start) / 1e6)
    s = sorted(times)
    return {
        "warmup": warmup, "samples": samples, "mean_ms": sum(s) / len(s),
        "p50_ms": _quantile(s, 0.5), "p90_ms": _quantile(s, 0.9), "p99_ms": _quantile(s, 0.99),
        "min_ms": s[0], "max_ms": s[-1],
    }


def write_result(path, *, framework, model, task, split, host, parameters, trainable,
                 peak_rss_bytes, idle_rss_bytes, device, threads, latency, quality,
                 higher_is_better, notes=None, peak_device_bytes=0):
    result = {
        "schema_version": SCHEMA_VERSION, "framework": framework, "model": model,
        "task": task, "split": split, "host": host,
        "parameters": parameters, "trainable": trainable,
        "peak_rss_bytes": peak_rss_bytes, "idle_rss_bytes": idle_rss_bytes,
        "device": device, "threads": threads, "latency": latency,
        "quality": quality, "higher_is_better": list(higher_is_better),
    }
    if peak_device_bytes:
        result["peak_device_bytes"] = peak_device_bytes
    if notes:
        result["notes"] = list(notes)
    # Exclusive create, like the Go side: never overwrite an earlier measurement.
    with open(path, "x") as f:
        json.dump(result, f, indent=2)
        f.write("\n")
    return result
