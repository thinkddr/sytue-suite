# SPDX-License-Identifier: Apache-2.0
"""Public ciphertext admission using the supported Synapse module callbacks."""
from pathlib import Path
from urllib.parse import urlsplit
import hashlib
import hmac
import json
import logging
import re
import secrets

from twisted.web.resource import Resource

from synapse.module_api import NOT_SPAM
from synapse.module_api.errors import Codes

_LOG = logging.getLogger(__name__)
_COVERAGE_KIND = "storage-resource-managed-v3"


def _allows_publication(decision):
    # Both callbacks must agree with the exact backend contract. Python booleans
    # are integers, so equality alone would accept an invalid protocol version.
    return (isinstance(decision, dict)
            and set(decision) == {"v", "coverage", "managed", "allow"}
            and type(decision["v"]) is int and decision["v"] == 1
            and decision["coverage"] == _COVERAGE_KIND
            and type(decision["managed"]) is bool
            and decision["allow"] is True)


class PublicationCoverage(Resource):
    """Fresh process-local witness; no user credentials or content are exposed."""
    isLeaf = True

    def __init__(self, server, secret):
        super().__init__()
        self._server = server
        self._secret = secret.encode("ascii")
        self._instance = secrets.token_hex(32)

    def render_GET(self, request):
        # The chart probes this exact module, rather than Synapse's generic health.
        request.setHeader(b"Cache-Control", b"no-store")
        request.setResponseCode(204)
        return b""

    def render_POST(self, request):
        request.setHeader(b"Cache-Control", b"no-store")
        request.setHeader(b"Content-Type", b"application/json")
        if not hmac.compare_digest((request.getHeader("Authorization") or "").encode("utf-8"),
                                   b"Bearer " + self._secret):
            request.setResponseCode(403)
            return b'{}'
        raw = request.content.read(1025)
        try:
            value = json.loads(raw) if len(raw) <= 1024 else None
            if (not isinstance(value, dict) or set(value) != {"v", "nonce"}
                    or type(value["v"]) is not int or value["v"] != 1
                    or not isinstance(value["nonce"], str)
                    or not re.fullmatch(r"[0-9a-f]{64}", value["nonce"])):
                raise ValueError("invalid coverage challenge")
        except (ValueError, TypeError, UnicodeError):
            request.setResponseCode(400)
            return b'{}'
        coverage = _COVERAGE_KIND
        transcript = ("sytue-publication-coverage-v1\n" + self._server + "\n"
                      + coverage + "\n" + self._instance + "\n" + value["nonce"])
        return json.dumps({"v": 1, "server": self._server, "coverage": coverage,
                           "instance": self._instance, "nonce": value["nonce"],
                           "mac": hmac.new(self._secret, transcript.encode("utf-8"),
                                           hashlib.sha256).hexdigest()}).encode("utf-8")


class PublicationAdmission:
    def __init__(self, config, api):
        self._api = api
        self._endpoint = config["endpoint"].rstrip("/")
        target = urlsplit(self._endpoint)
        if (target.scheme not in ("http", "https") or not target.hostname
                or target.username or target.password or target.query or target.fragment
                or target.path != "/internal/matrix/publications"):
            raise ValueError("invalid publication admission endpoint")
        self._server = config["server_name"]
        if not isinstance(self._server, str) or not self._server:
            raise ValueError("publication admission server is required")
        secret = Path(config["secret_file"]).read_text(encoding="ascii").strip()
        if not re.fullmatch(r"[0-9a-f]{64}", secret):
            raise ValueError("invalid publication admission secret")
        self._headers = {b"Authorization": [b"Bearer " + secret.encode("ascii")]}
        api.register_spam_checker_callbacks(check_event_for_spam=self.check_event_for_spam)
        api.register_third_party_rules_callbacks(on_new_event=self.on_new_event)
        api.register_web_resource("/_synapse/client/sytue/publication",
                                  PublicationCoverage(self._server, secret))

    @staticmethod
    def parse_config(config):
        required = {"endpoint", "secret_file", "server_name"}
        if not isinstance(config, dict) or set(config) != required:
            raise ValueError("publication admission configuration is incomplete")
        return config

    def _payload(self, event):
        if event.type not in ("m.room.encrypted", "m.room.message"):
            return None
        if event.sender.partition(":")[2] != self._server:
            return None
        payload = {
            "v": 1,
            "reference": {"room_id": event.room_id, "event_id": event.event_id},
            "sender": event.sender,
            "type": event.type,
        }
        if event.type == "m.room.encrypted":
            payload["content"] = event.get_dict()["content"]
        return payload

    async def _post(self, suffix, payload):
        return await self._api.http_client.post_json_get_json(
            self._endpoint + suffix, payload, headers=self._headers
        )

    async def check_event_for_spam(self, event):
        try:
            payload = self._payload(event)
            if payload is None:
                return NOT_SPAM
            decision = await self._post("/before", payload)
            if _allows_publication(decision):
                return NOT_SPAM
        except Exception:
            # Never log the request, authorization, response or private content.
            _LOG.warning("publication admission unavailable")
        return Codes.FORBIDDEN

    async def on_new_event(self, event, state_events):
        if event.type != "m.room.encrypted":
            return
        payload = self._payload(event)
        if payload is None:
            return
        try:
            decision = await self._post("/accepted", payload)
            if not _allows_publication(decision):
                _LOG.warning("publication acknowledgement unavailable")
        except Exception:
            # The durable sending record remains pending for reconciliation.
            _LOG.warning("publication acknowledgement unavailable")
