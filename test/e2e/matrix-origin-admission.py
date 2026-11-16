#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Remote-only, disposable Synapse module boundary proof; no product sessions."""
import hashlib
import hmac
import json
import os
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import Request, urlopen

assert os.environ.get("SYTUE_MATRIX_ORIGIN_FIXTURE") == "1", "disposable remote fixture required"
BASE = "http://127.0.0.1:18978"
SECRET = "0" * 64  # Artificial fixture only.
records = []
lock = threading.Lock()

def verify_callback_contract():
    # Run inside the same pinned Synapse image as the live HTTP proof. Exercise
    # the actual callbacks, including accepted replies, without a fake module.
    import asyncio
    import logging
    from types import SimpleNamespace
    from synapse.module_api import NOT_SPAM
    from synapse.module_api.errors import Codes
    from sytue_publication import PublicationAdmission

    good = {"v": 1, "coverage": "storage-resource-managed-v3", "managed": True, "allow": True}
    bad = [None, [], {}, dict(good, v=True), dict(good, v=1.0),
           dict(good, managed=1), dict(good, allow=1), dict(good, allow=False),
           dict(good, coverage="storage-private-v2"), dict(good, extra=False)]
    for missing in good:
        bad.append({key: value for key, value in good.items() if key != missing})
    event = SimpleNamespace(type="m.room.encrypted", sender="@fixture:storage.test:18978",
                            room_id="!fixture:storage.test:18978", event_id="$fixture",
                            get_dict=lambda: {"content": {"ciphertext": "artificial"}})
    class Capture(logging.Handler):
        def __init__(self):
            super().__init__(); self.messages = []
        def emit(self, record):
            self.messages.append(record.getMessage())
    capture = Capture()
    logger = logging.getLogger("sytue_publication")
    logger.addHandler(capture)
    async def run():
        admission = object.__new__(PublicationAdmission)
        admission._server = "storage.test:18978"
        for decision, valid in [(good, True), (dict(good, managed=False), True)] + [(value, False) for value in bad]:
            calls = []
            async def post(suffix, payload):
                calls.append(suffix)
                return decision
            admission._post = post
            capture.messages.clear()
            before = await admission.check_event_for_spam(event)
            assert before == (NOT_SPAM if valid else Codes.FORBIDDEN), "before contract changed"
            await admission.on_new_event(event, {})
            assert calls == ["/before", "/accepted"]
            assert capture.messages == ([] if valid else ["publication acknowledgement unavailable"]), "accepted contract changed"
    try:
        asyncio.run(run())
    finally:
        logger.removeHandler(capture)
    print("ACTUAL_SYNAPSE_CALLBACK_CONTRACT_GREEN before and accepted reject mixed versions and malformed fields", flush=True)

if sys.argv[1:] == ["--callback-contract"]:
    verify_callback_contract()
    raise SystemExit(0)

class Control(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        assert 0 < length <= 131072
        payload = json.loads(self.rfile.read(length))
        assert self.headers.get("Authorization") == "Bearer " + SECRET
        assert set(payload) <= {"v", "reference", "sender", "type", "content"}
        assert payload["v"] == 1
        assert set(payload["reference"]) == {"room_id", "event_id"}
        if payload["type"] == "m.room.message":
            assert "content" not in payload, "plaintext reached control plane"
        case = payload.get("content", {}).get("ciphertext", "plaintext")
        with lock:
            records.append((self.path, payload))
        status = 200
        decision = {"v": 1, "managed": True, "allow": True, "coverage": "storage-resource-managed-v3"}
        if self.path.endswith("/before"):
            if case in ("deny", "plaintext"):
                decision["allow"] = False
            elif case == "unavailable":
                status = 503
            elif case == "not-a-boolean":
                decision["allow"] = 1
            elif case == "old-backend":
                del decision["coverage"]
            elif case == "not-managed-boolean":
                decision["managed"] = 1
            elif case == "boolean-version":
                decision["v"] = True
            elif case == "storage-only-v2":
                decision["coverage"] = "storage-private-v2"
            elif case == "extra-field":
                decision["extra"] = False
        elif self.path.endswith("/accepted"):
            if case == "lost-ack":
                status = 503
        else:
            status = 404
        body = json.dumps(decision).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

def call(method, path, body=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    raw = None if body is None else json.dumps(body).encode()
    try:
        with urlopen(Request(BASE + path, data=raw, headers=headers, method=method), timeout=15) as response:
            return response.status, json.load(response)
    except HTTPError as error:
        return error.code, json.load(error)

def matching(suffix, case):
    with lock:
        return [value for path, value in records
                if path.endswith(suffix) and value.get("content", {}).get("ciphertext", "plaintext") == case]

def await_accepted(case):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        values = matching("/accepted", case)
        if values:
            return values[-1]
        time.sleep(0.05)
    raise AssertionError("persisted event was not acknowledged")

control = ThreadingHTTPServer(("127.0.0.1", 18979), Control)
thread = threading.Thread(target=control.serve_forever, daemon=True)
thread.start()
try:
    # The live module is authenticated independently of Matrix user/admin tokens.
    coverage_path = "/_synapse/client/sytue/publication"
    with urlopen(BASE + coverage_path, timeout=15) as response:
        assert response.status == 204 and response.headers.get("Cache-Control") == "no-store"
    instance = None
    for nonce in ("a" * 64, "b" * 64):
        code, witness = call("POST", coverage_path, {"v": 1, "nonce": nonce}, SECRET)
        assert code == 200 and witness["v"] == 1
        assert witness["server"] == "storage.test:18978"
        assert witness["coverage"] == "storage-resource-managed-v3" and witness["nonce"] == nonce
        assert len(witness["instance"]) == 64
        assert instance is None or witness["instance"] == instance
        instance = witness["instance"]
        transcript = "sytue-publication-coverage-v1\n" + witness["server"] + "\n" + witness["coverage"] + "\n" + instance + "\n" + nonce
        assert hmac.compare_digest(witness["mac"], hmac.new(SECRET.encode(), transcript.encode(), hashlib.sha256).hexdigest())
    for token in (None, "1" * 64):
        assert call("POST", coverage_path, {"v": 1, "nonce": "a" * 64}, token)[0] == 403
    for bad in ({"v": True, "nonce": "a" * 64}, {"v": 1, "nonce": "A" * 64}, {"v": 1, "nonce": "short"}, {"v": 1, "nonce": "a" * 64, "extra": 1}):
        assert call("POST", coverage_path, bad, SECRET)[0] == 400
    code, session = call("POST", "/_matrix/client/v3/register", {
        "username": "origin_fixture", "password": "artificial-disposable-fixture",
        "auth": {"type": "m.login.dummy"}, "device_id": "ORIGIN_FIXTURE"
    })
    assert code == 200, ("fixture registration", code)
    token = session["access_token"]
    code, created = call("POST", "/_matrix/client/v3/createRoom", {
        "preset": "private_chat",
        "initial_state": [{"type": "m.room.encryption", "state_key": "",
                           "content": {"algorithm": "m.megolm.v1.aes-sha2"}}]
    }, token)
    assert code == 200, ("room creation", code)
    room = created["room_id"]
    path = "/_matrix/client/v3/rooms/" + quote(room, safe="") + "/send/"
    for case, expected in (("allow", 200), ("deny", 403), ("unavailable", 403),
                           ("not-a-boolean", 403), ("not-managed-boolean", 403), ("old-backend", 403),
                           ("boolean-version", 403), ("storage-only-v2", 403), ("extra-field", 403), ("lost-ack", 200)):
        content = {"algorithm": "m.megolm.v1.aes-sha2", "session_id": "synthetic-session",
                   "ciphertext": case, "org.sytue.publication.admission": {"v": 1, "id": "A" * 43}}
        code, sent = call("PUT", path + "m.room.encrypted/" + case, content, token)
        assert code == expected, ("origin decision", case, code)
        before = matching("/before", case)
        assert len(before) == 1 and before[0]["content"] == content, "wire changed before admission"
        if expected == 200:
            accepted = await_accepted(case)
            assert accepted["reference"]["event_id"] == sent["event_id"], "pre-persist event ID changed"
            assert before[0]["reference"] == accepted["reference"], "origin identity changed"
            code, event = call("GET", "/_matrix/client/v3/rooms/" + quote(room, safe="") +
                               "/event/" + quote(sent["event_id"], safe=""), token=token)
            assert code == 200 and event["content"] == content, "persisted wire changed"
            # The SDK transaction retry must use the existing event, even when
            # the internal acknowledgement response was lost.
            code, retry = call("PUT", path + "m.room.encrypted/" + case, content, token)
            assert code == 200 and retry["event_id"] == sent["event_id"]
            assert len(matching("/before", case)) == 1, "transaction retry re-admitted content"
        else:
            assert not matching("/accepted", case), "denied content persisted"
    code, _ = call("PUT", path + "m.room.message/plaintext",
                   {"msgtype": "m.text", "body": "plaintext-must-never-reach-control-plane"}, token)
    assert code == 403 and len(matching("/before", "plaintext")) == 1
    assert "content" not in matching("/before", "plaintext")[0]
    assert not matching("/accepted", "plaintext")
    go_root = os.environ.get("SYTUE_MATRIX_ORIGIN_GO_ROOT")
    if go_root:
        env = dict(os.environ, PATH="/usr/local/go/bin:/usr/local/bin:/root/go/bin:/usr/bin:/bin",
                   GOTOOLCHAIN="local", GOFLAGS="-mod=readonly -p=1", GOMAXPROCS="2", GOMEMLIMIT="3GiB")
        subprocess.run(["go", "test", "-race", "-tags=integration,ci,goolm,migrated_fynedo",
                        "./services/rooms/internal/matrix", "-run", "^(TestStoragePublicationOriginRealSynapseReadsAfterProducerLogout|TestPublicationCoverageRealSynapseFreshProcessWitness|TestRetainedPublicationOriginRealSynapseCanonicalReadAfterLogout)$",
                        "-count=1", "-timeout=2m", "-v"], cwd=go_root, env=env, check=True, timeout=150)
    print("REAL_SYNAPSE_ORIGIN_MODULE_BOUNDARY_GREEN 11 publication cases plus authenticated coverage; synthetic ciphertext only", flush=True)
finally:
    control.shutdown()
    control.server_close()
    thread.join(timeout=5)
