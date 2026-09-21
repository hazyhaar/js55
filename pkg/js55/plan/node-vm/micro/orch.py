#!/usr/bin/env python3
"""Client léger : N POST concurrents vers vLLM, écriture locale des .c."""

from __future__ import annotations

import json
import os
import re
import sys
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

VLLM_URL = os.environ.get("VLLM_URL", "http://127.0.0.1:8000/v1/chat/completions")
MODEL = os.environ.get("VLLM_MODEL", "ornith-1.5")
CONCURRENCY = int(os.environ.get("ORCH_CONCURRENCY", "12"))
MAX_TOKENS = int(os.environ.get("ORCH_MAX_TOKENS", "4096"))

WRITE_RE = re.compile(r"Write\s+`(/devhoros/c2simd/sources/c2archtsim/[^`]+\.c)`")
CFILE_RE = re.compile(r"<c_file\s+name=\"([^\"]+)\">(.*?)</c_file>", re.DOTALL)
THINK_RE = re.compile(r"<think>.*?</think>", re.DOTALL)
FENCE_RE = re.compile(r"```(?:c|C)?\n(.*?)```", re.DOTALL)

SYSTEM = (
    "Tu génères un fichier C à partir de la fiche. Réponse strictement :\n"
    '<c_file name="nom_du_fichier.c">\n'
    "/* code C complet */\n"
    "</c_file>\n"
    "Pas de prose hors de la balise. Pas d'appel d'outil."
)


def extract_dest(fiche: Path) -> Path | None:
    m = WRITE_RE.search(fiche.read_text(encoding="utf-8"))
    if not m:
        return None
    return Path(m.group(1))


def strip_think(text: str) -> str:
    return THINK_RE.sub("", text or "").strip()


def extract_c(raw: str, dest: Path) -> str:
    text = strip_think(raw)
    m = CFILE_RE.search(text)
    if m:
        return m.group(2).strip() + "\n"
    m = FENCE_RE.search(text)
    if m:
        return m.group(1).strip() + "\n"
    i = text.find("// SPDX-License-Identifier")
    if i >= 0:
        return text[i:].strip() + "\n"
    raise ValueError(f"pas de C extractible pour {dest.name}")


def complete(fiche_text: str) -> str:
    payload = {
        "model": MODEL,
        "messages": [
            {"role": "system", "content": SYSTEM},
            {"role": "user", "content": fiche_text},
        ],
        "temperature": 0.1,
        "max_tokens": MAX_TOKENS,
        "stop": ["<|im_end|>"],
    }
    req = urllib.request.Request(
        VLLM_URL,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=180) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    msg = data["choices"][0]["message"]
    content = msg.get("content") or ""
    reasoning = msg.get("reasoning") or ""
    return (reasoning + "\n" + content).strip()


def run_one(fiche: Path, dest: Path, force: bool) -> str:
    if dest.exists() and not force:
        prev = dest.read_text(encoding="utf-8", errors="replace")
        if "SPDX-License-Identifier" in prev and "c2_v8_" in prev and "#include" in prev:
            return f"skip {dest.name}"
    dest.parent.mkdir(parents=True, exist_ok=True)
    raw = complete(fiche.read_text(encoding="utf-8"))
    if not raw.rstrip().endswith("</c_file>"):
        raw = raw + "\n</c_file>"
    body = extract_c(raw, dest)
    if "SPDX-License-Identifier" not in body or "c2_v8_" not in body or "#include" not in body:
        Path("/tmp").joinpath(f"orch-fail-{dest.name}.txt").write_text(raw[:8000], encoding="utf-8")
        raise ValueError(f"sortie non-C ({len(body)} o)")
    tmp = dest.with_suffix(".c.tmp")
    tmp.write_text(body, encoding="utf-8")
    tmp.replace(dest)
    return f"ok {dest.name} ({len(body)} o)"


def main() -> int:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else Path(__file__).resolve().parent)
    force = "--force" in sys.argv
    jobs: list[tuple[Path, Path]] = []
    for fiche in sorted(root.glob("C*.md")):
        dest = extract_dest(fiche)
        if dest is None:
            continue
        jobs.append((fiche, dest))
    if not jobs:
        print("aucune fiche C*.md", file=sys.stderr)
        return 1
    print(f"{len(jobs)} fiches, concurrence {CONCURRENCY}", flush=True)
    failed = 0
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as pool:
        futs = {pool.submit(run_one, f, d, force): (f, d) for f, d in jobs}
        for fut in as_completed(futs):
            fiche, dest = futs[fut]
            try:
                print(fut.result(), flush=True)
            except Exception as exc:
                failed += 1
                print(f"fail {fiche.name} -> {dest.name}: {exc}", file=sys.stderr, flush=True)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
