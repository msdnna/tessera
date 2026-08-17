#!/usr/bin/env python3
"""HTTP wrapper around headless LibreOffice.

The service has no authentication of its own and converts whatever it is
handed, so it must stay on the internal compose network with no published port.
Tessera's backend is the only client (backend/internal/converter).

Two directions, both driven by the Documents section (#2733):

  import   .docx/.odt/... -> html, which the frontend parses with the editor's
           own TipTap schema (frontend/src/utils/docImport.js). Parsing there
           rather than here is deliberate: the schema *is* the allow-list, so a
           second HTML->blocks walk on this side would be a worse converter that
           drifts from the first one.
  export   html -> .docx/.pdf, where the HTML comes from the same editor.

Endpoints:
  GET  /health              -> {"ok": true, "formats": {...}}
  POST /convert?from=&to=   -> converted bytes, request body is the source file
"""

import base64
import mimetypes
import os
import re
import subprocess
import sys
import tempfile
import urllib.parse
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(os.environ.get("CONVERTER_PORT", "3000"))

# Wall-clock ceiling for one soffice run. LibreOffice can wedge on a malformed
# document, and a hung request would otherwise hold a worker thread forever.
CONVERT_TIMEOUT = int(os.environ.get("CONVERT_TIMEOUT_SEC", "120"))

# Refused before anything is written to disk. The backend caps uploads lower
# still (maxDocImportBytes); this is the last line, not the policy.
MAX_BODY_BYTES = int(os.environ.get("CONVERT_MAX_BYTES", str(32 * 1024 * 1024)))

# What LibreOffice is asked to produce. The filter name matters: bare "html"
# picks the Calc/Impress filter for some inputs, and "HTML (StarWriter)" pins
# the Writer one.
TARGETS = {
    "html": ("html:HTML (StarWriter)", "html", "text/html; charset=utf-8"),
    "pdf": ("pdf:writer_pdf_Export", "pdf", "application/pdf"),
    "docx": (
        "docx:MS Word 2007 XML",
        "docx",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    ),
    "odt": ("odt:writer8", "odt", "application/vnd.oasis.opendocument.text"),
}

# Accepted source extensions. Kept explicit rather than "whatever soffice
# opens": the list is echoed to clients from /health and shown in the file
# picker, so it has to be something we can state, not discover.
SOURCES = {
    "doc",
    "docx",
    "odt",
    "rtf",
    "fodt",
    "html",
    "htm",
    "txt",
    "md",
}

_SAFE_EXT = re.compile(r"^[a-z0-9]{1,8}$")

# LibreOffice's HTML export writes pictures next to the document as
# input_html_<hash>.<ext> and links them relatively. A file on the backend's
# disk is useless to us — the caller gets bytes, not a directory — so every
# referenced picture is folded back into the markup as a data: URI and the
# backend turns those into document assets.
_IMG_SRC = re.compile(r'(<img\b[^>]*?\bsrc=")([^"]+)(")', re.IGNORECASE)


def _inline_images(html: str, outdir: str) -> str:
    """Replaces relative <img src> with data: URIs read from outdir."""

    def repl(m):
        src = m.group(2)
        if "://" in src or src.startswith("data:"):
            return m.group(0)
        path = os.path.join(outdir, os.path.basename(urllib.parse.unquote(src)))
        try:
            with open(path, "rb") as fh:
                raw = fh.read()
        except OSError:
            # A picture LibreOffice referenced but did not write. Dropping the
            # src leaves a visible broken image rather than a data: URI of
            # nothing, which is the honest outcome for the person importing.
            return m.group(0)
        mime = mimetypes.guess_type(path)[0] or "application/octet-stream"
        return m.group(1) + "data:" + mime + ";base64," + base64.b64encode(raw).decode("ascii") + m.group(3)

    return _IMG_SRC.sub(repl, html)


class ConvertError(Exception):
    """A request that cannot be served, with the HTTP status to answer with."""

    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status
        self.message = message


def convert(data: bytes, src_ext: str, target: str) -> bytes:
    if target not in TARGETS:
        raise ConvertError(400, "unsupported target format: " + target)
    if src_ext not in SOURCES:
        raise ConvertError(415, "unsupported source format: " + src_ext)

    convert_to, out_ext, _ = TARGETS[target]
    with tempfile.TemporaryDirectory(prefix="conv-") as tmp:
        src = os.path.join(tmp, "input." + src_ext)
        with open(src, "wb") as fh:
            fh.write(data)

        # A private UserInstallation per request. Without it concurrent soffice
        # invocations share one profile directory, and the second one exits
        # immediately with "another instance is running" instead of converting.
        profile = "file://" + os.path.join(tmp, "profile-" + uuid.uuid4().hex)
        cmd = [
            "soffice",
            "--headless",
            "--norestore",
            "--nolockcheck",
            "-env:UserInstallation=" + profile,
            "--convert-to",
            convert_to,
            "--outdir",
            tmp,
            src,
        ]
        try:
            proc = subprocess.run(cmd, capture_output=True, timeout=CONVERT_TIMEOUT)
        except subprocess.TimeoutExpired:
            raise ConvertError(504, "conversion timed out after %ds" % CONVERT_TIMEOUT)

        out = os.path.join(tmp, "input." + out_ext)
        if not os.path.exists(out):
            # soffice exits 0 on inputs it cannot parse and simply writes
            # nothing, so the exit code is not the signal — the missing file is.
            detail = (proc.stderr or proc.stdout or b"").decode("utf-8", "replace").strip()
            raise ConvertError(422, "LibreOffice produced no output" + (": " + detail[:400] if detail else ""))

        with open(out, "rb") as fh:
            raw = fh.read()

        if target == "html":
            text = raw.decode("utf-8", "replace")
            return _inline_images(text, tmp).encode("utf-8")
        return raw


class Handler(BaseHTTPRequestHandler):
    server_version = "tessera-converter"

    def log_message(self, fmt, *args):  # noqa: A003 - BaseHTTPRequestHandler API
        sys.stderr.write("converter: " + (fmt % args) + "\n")

    def _send(self, status: int, body: bytes, content_type: str):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _fail(self, status: int, message: str):
        payload = ('{"error":%s}' % _json_string(message)).encode("utf-8")
        self._send(status, payload, "application/json")

    def do_GET(self):
        path = urllib.parse.urlparse(self.path).path
        if path != "/health":
            self._fail(404, "not found")
            return
        body = (
            '{"ok":true,"sources":[%s],"targets":[%s]}'
            % (
                ",".join(_json_string(s) for s in sorted(SOURCES)),
                ",".join(_json_string(t) for t in sorted(TARGETS)),
            )
        ).encode("utf-8")
        self._send(200, body, "application/json")

    def do_POST(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path != "/convert":
            self._fail(404, "not found")
            return

        query = urllib.parse.parse_qs(parsed.query)
        src_ext = (query.get("from", [""])[0] or "").lower().lstrip(".")
        target = (query.get("to", [""])[0] or "").lower()
        if not _SAFE_EXT.match(src_ext or ""):
            self._fail(400, "missing or malformed 'from'")
            return

        try:
            length = int(self.headers.get("Content-Length") or 0)
        except ValueError:
            self._fail(400, "malformed Content-Length")
            return
        if length <= 0:
            self._fail(400, "empty body")
            return
        if length > MAX_BODY_BYTES:
            self._fail(413, "body larger than %d bytes" % MAX_BODY_BYTES)
            return

        data = self.rfile.read(length)
        try:
            out = convert(data, src_ext, target)
        except ConvertError as err:
            self._fail(err.status, err.message)
            return
        except Exception as err:  # pragma: no cover - defensive
            self._fail(500, "conversion failed: %s" % err)
            return

        self._send(200, out, TARGETS[target][2])


def _json_string(s: str) -> str:
    out = ['"']
    for ch in s:
        if ch == '"':
            out.append('\\"')
        elif ch == "\\":
            out.append("\\\\")
        elif ch == "\n":
            out.append("\\n")
        elif ch == "\r":
            out.append("\\r")
        elif ch == "\t":
            out.append("\\t")
        elif ord(ch) < 0x20:
            out.append("\\u%04x" % ord(ch))
        else:
            out.append(ch)
    out.append('"')
    return "".join(out)


def main():
    server = ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    sys.stderr.write("converter: listening on :%d\n" % PORT)
    server.serve_forever()


if __name__ == "__main__":
    main()
