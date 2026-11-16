#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Exercise network retries and the verified public AOSP source cache."""
import base64
from email.message import Message
import hashlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock
import urllib.error

spec = importlib.util.spec_from_file_location("android_work_context", Path(__file__).with_name("android-work-context-test.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
DATA = b"synthetic public source bytes"
DIGEST = hashlib.sha256(DATA).hexdigest()


def failure(code, after=None):
    headers = Message()
    if after is not None:
        headers["Retry-After"] = after
    return urllib.error.HTTPError(module.SOURCE, code, "synthetic test response", headers, None)


class DownloadTests(unittest.TestCase):
    def test_rate_limit_then_success_respects_retry_after(self):
        with mock.patch.object(module.urllib.request, "urlopen", side_effect=[failure(429, "2"), io.BytesIO(base64.b64encode(DATA))]) as request, mock.patch.object(module.time, "sleep") as sleep:
            self.assertEqual(module.download_source("source"), DATA)
            self.assertEqual(request.call_count, 2)
            sleep.assert_called_once_with(2)

    def test_retry_after_dates_are_bounded_and_invalid_headers_use_backoff(self):
        headers = Message()
        headers["Retry-After"] = "Thu, 01 Jan 1970 00:01:40 GMT"
        with mock.patch.object(module.time, "time", return_value=10):
            self.assertEqual(module.retry_delay(headers, 0), 5)
        headers.replace_header("Retry-After", "invalid")
        self.assertEqual(module.retry_delay(headers, 1), 2)
        headers.replace_header("Retry-After", "9999999999")
        self.assertEqual(module.retry_delay(headers, 0), 5)
        headers.replace_header("Retry-After", "0")
        self.assertEqual(module.retry_delay(headers, 0), 0)

    def test_exhaustion_preserves_the_http_error(self):
        with mock.patch.object(module.urllib.request, "urlopen", side_effect=[failure(503) for _ in range(4)]) as request, mock.patch.object(module.time, "sleep") as sleep:
            with self.assertRaises(urllib.error.HTTPError) as raised:
                module.download_source("source")
            self.assertEqual(raised.exception.code, 503)
            self.assertEqual(request.call_count, 4)
            self.assertEqual(sleep.call_args_list, [mock.call(1), mock.call(2), mock.call(4)])

    def test_404_and_non_transient_failures_are_not_hidden(self):
        for error in [failure(404), urllib.error.URLError(ValueError("not transient"))]:
            with self.subTest(error=error), mock.patch.object(module.urllib.request, "urlopen", side_effect=error) as request, mock.patch.object(module.time, "sleep") as sleep:
                with self.assertRaises(type(error)):
                    module.download_source("source")
                self.assertEqual(request.call_count, 1)
                sleep.assert_not_called()

    def test_transient_network_timeout_is_retried(self):
        with mock.patch.object(module.urllib.request, "urlopen", side_effect=[urllib.error.URLError(TimeoutError()), io.BytesIO(base64.b64encode(DATA))]), mock.patch.object(module.time, "sleep") as sleep:
            self.assertEqual(module.download_source("source"), DATA)
            sleep.assert_called_once_with(1)

    def test_corrupt_download_is_not_retried_or_cached(self):
        with tempfile.TemporaryDirectory() as tmp, mock.patch.object(module.urllib.request, "urlopen", return_value=io.BytesIO(base64.b64encode(b"wrong bytes"))) as request, mock.patch.object(module.time, "sleep") as sleep:
            with self.assertRaisesRegex(ValueError, "source hash mismatch"):
                module.source_bytes("fixture", "source", DIGEST, Path(tmp))
            self.assertEqual(list(Path(tmp).iterdir()), [])
            self.assertEqual(request.call_count, 1)
            sleep.assert_not_called()

    def test_replay_checks_hash_and_never_truncates_or_refetches_valid_cache(self):
        with tempfile.TemporaryDirectory() as tmp:
            cache = Path(tmp)
            with mock.patch.object(module, "download_source", return_value=DATA) as request:
                self.assertEqual(module.source_bytes("fixture", "source", DIGEST, cache), DATA)
                request.assert_called_once_with("source")
            self.assertEqual([p.name for p in cache.iterdir()], [DIGEST + ".java"])
            with mock.patch.object(module, "download_source", side_effect=AssertionError("network on cache hit")):
                self.assertEqual(module.source_bytes("fixture", "source", DIGEST, cache), DATA)
                (cache / (DIGEST + ".java")).write_bytes(b"corrupt cached source")
                with self.assertRaisesRegex(ValueError, "source hash mismatch"):
                    module.source_bytes("fixture", "source", DIGEST, cache)
                self.assertEqual((cache / (DIGEST + ".java")).read_bytes(), b"corrupt cached source")


if __name__ == "__main__":
    unittest.main()
