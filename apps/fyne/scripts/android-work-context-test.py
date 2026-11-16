#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Cross-check Go's work identity against the actual pinned Android JSON writer.

The public android.jar contains method stubs, so it cannot test JSON encoding.
Run untouched AOSP16 org.json sources instead. Only two unavailable annotations
are SOURCE-only compiler declarations; no JSON/runtime implementation is mocked.
All third-party sources stay in a temporary directory and are hash-checked before
compilation. No Android SDK installation or license acceptance is required.
"""
import argparse
import base64
import errno
from email.utils import parsedate_to_datetime
import datetime
import hashlib
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

FYNE = Path(__file__).resolve().parents[1]
SOURCE = "https://android.googlesource.com/platform/libcore/+/refs/tags/android-16.0.0_r1/"
FILES = {
    "JSON": ("json/src/main/java/org/json/JSON.java", "a2cc5e4bf64bfe3edd73c6a14409f3200640454051d9947ee79f16a2396a19b1"),
    "JSONArray": ("json/src/main/java/org/json/JSONArray.java", "24f10dbe57a9228b8aec8a891355c26285e7fc4e070a2f826a080300e3ca4180"),
    "JSONException": ("json/src/main/java/org/json/JSONException.java", "26e10aae9c209dabbc8f15b4156fe9fdc87f5d9d2cb2d2f078b475a61ba0df7d"),
    "JSONObject": ("json/src/main/java/org/json/JSONObject.java", "b7770e43fdad8415e8f5c37a7dc06a7c1ffb3a16c557887ded1479f3116c868f"),
    "JSONStringer": ("json/src/main/java/org/json/JSONStringer.java", "8772ac80610cd62e906330152b09864c471a0921f199f4b9d670d98050e3d86c"),
    "JSONTokener": ("json/src/main/java/org/json/JSONTokener.java", "744c9ffb50c2472d7269c8d64a7d8331ea1a1691fb6536619ac9590e9ebbbd4b"),
    "IntraCoreApi": ("luni/src/main/java/libcore/api/IntraCoreApi.java", "038ffa562831ecffb5a354294b852e01a7a87023d3df7f7505bc33480178ed1a"),
    "NonNull": ("luni/src/main/java/libcore/util/NonNull.java", "26014a46096363d9fac392ac12a227c4795dd95db72530abd94746f4c1067cfc"),
    "Nullable": ("luni/src/main/java/libcore/util/Nullable.java", "0bde695a15d21a3735a24e61d6c0795123607b97df7d6a09193a563241a27afc"),
}



MAX_ATTEMPTS = 4
MAX_RETRY_DELAY = 5
TRANSIENT_STATUS = {429, 500, 502, 503, 504}


def retry_delay(headers, attempt):
    value = "" if headers is None else headers.get("Retry-After", "").strip()
    if value.isascii() and value.isdigit() and len(value) <= 10:
        return min(MAX_RETRY_DELAY, int(value))
    if value:
        try:
            when = parsedate_to_datetime(value)
            if when.tzinfo is None:
                when = when.replace(tzinfo=datetime.timezone.utc)
            return min(MAX_RETRY_DELAY, max(0, when.timestamp() - time.time()))
        except (TypeError, ValueError, OverflowError):
            pass
    return min(MAX_RETRY_DELAY, 2 ** attempt)


def download_source(path):
    for attempt in range(MAX_ATTEMPTS):
        try:
            with urllib.request.urlopen(SOURCE + path + "?format=TEXT", timeout=30) as response:
                return base64.b64decode(response.read(2_000_000), validate=True)
        except urllib.error.HTTPError as error:
            if error.code not in TRANSIENT_STATUS or attempt == MAX_ATTEMPTS - 1:
                raise
            delay = retry_delay(error.headers, attempt)
        except (TimeoutError, ConnectionError, urllib.error.URLError) as error:
            reason = error.reason if isinstance(error, urllib.error.URLError) else error
            transient = isinstance(reason, (TimeoutError, ConnectionError)) or getattr(reason, "errno", None) in {
                errno.EAGAIN, socket.EAI_AGAIN, errno.ECONNRESET, errno.ECONNREFUSED, errno.ETIMEDOUT, errno.ENETUNREACH,
            }
            if not transient or attempt == MAX_ATTEMPTS - 1:
                raise
            delay = retry_delay(None, attempt)
        time.sleep(delay)
    raise AssertionError("unreachable source retry state")


def source_bytes(name, path, digest, cache):
    cached = None if cache is None else cache / (digest + ".java")
    hit = cached is not None and cached.is_file()
    if hit:
        with cached.open("rb") as source:
            data = source.read(2_000_001)
    else:
        data = download_source(path)
    if hashlib.sha256(data).hexdigest() != digest:
        raise ValueError("AOSP source hash mismatch: " + name)
    if cached is not None and not hit:
        # Concurrent CI jobs see either the complete verified source or no hit.
        with tempfile.NamedTemporaryFile(dir=cache, delete=False) as output:
            temporary = Path(output.name)
            output.write(data)
        try:
            os.replace(temporary, cached)
        finally:
            temporary.unlink(missing_ok=True)
    return data

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-cache", type=Path, help="reuse public sources; every read still checks its pinned hash")
    cache = parser.parse_args().source_cache
    if cache is not None:
        cache.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="sytue-android-work-json-") as temporary:
        root = Path(temporary)
        sources = []
        for name, (path, digest) in FILES.items():
            data = source_bytes(name, path, digest, cache)
            source = root / (name + ".java")
            source.write_bytes(data)
            sources.append(str(source))
        declarations = {
            "SystemApi": "package android.annotation; @java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.SOURCE) public @interface SystemApi { enum Client { MODULE_LIBRARIES } Client client(); }",
            "UnsupportedAppUsage": "package android.compat.annotation; @java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.SOURCE) public @interface UnsupportedAppUsage {}",
        }
        for name, text in declarations.items():
            source = root / (name + ".java")
            source.write_text(text)
            sources.append(str(source))
        classes = root / "classes"
        # Platform sources have upstream unchecked/raw/legacy API warnings.
        subprocess.run(["javac", "--release", "17", "-encoding", "UTF-8", "-Xlint:none", "-d", str(classes), *sources], check=True)
        subprocess.run(["javac", "--release", "17", "-encoding", "UTF-8", "-Xlint:all", "-Werror", "-cp", str(classes), "-d", str(classes),
                        str(FYNE / "android/java/eu/sytue/nativo/SesionContextoTrabajo.java"),
                        str(FYNE / "android/prueba/eu/sytue/nativo/SesionContextoTrabajoPrueba.java")], check=True)
        subprocess.run(["java", "-cp", str(classes), "eu.sytue.nativo.SesionContextoTrabajoPrueba",
                        str(FYNE.parent / "nucleo/auth/testdata/android-work-context.json")], check=True)


if __name__ == "__main__":
    main()
