#!/usr/bin/env python3
"""Tests for the conversion sidecar's request handling.

Run from this directory:

    python3 -m unittest server_test -v

soffice is stubbed throughout — the point of these is the wrapper's own logic
(which request is refused, which file is read back), not LibreOffice. A real
conversion is exercised by the Documents import path in backend e2e.
"""
import os
import unittest

import server


class FakeProc:
    """What subprocess.run returns for a stubbed soffice."""

    def __init__(self, returncode=0, stdout=b"", stderr=b""):
        self.returncode = returncode
        self.stdout = stdout
        self.stderr = stderr


class SofficeStub:
    """Stands in for soffice: writes `writes` into --outdir, then reports `proc`.

    `writes` maps output file name -> bytes. An empty mapping is the failure the
    "produced no output" guard exists for: LibreOffice exits 0 on an input it
    cannot parse and simply writes nothing.
    """

    def __init__(self, writes=None, proc=None):
        self.writes = writes or {}
        self.proc = proc or FakeProc(stderr=b"Error: source file could not be loaded")
        self.calls = []

    def __call__(self, cmd, **kwargs):
        self.calls.append(cmd)
        outdir = cmd[cmd.index("--outdir") + 1]
        for name, body in self.writes.items():
            with open(os.path.join(outdir, name), "wb") as fh:
                fh.write(body)
        return self.proc


class ConvertTest(unittest.TestCase):
    def setUp(self):
        self._run = server.subprocess.run

    def tearDown(self):
        server.subprocess.run = self._run

    def _stub(self, writes=None, proc=None):
        stub = SofficeStub(writes, proc)
        server.subprocess.run = stub
        return stub

    def test_no_output_is_refused_for_every_source_format(self):
        """A run that wrote nothing is a 422 — including html->html.

        The output path used to be built in the same directory as the source and
        from the same stem, so for a source whose extension already equals the
        target the guard was reading the *input* file back: an .html import whose
        conversion produced nothing answered 200 with the raw uploaded bytes.
        """
        body = b"<html><body><p>hello</p></body></html>"
        for ext in sorted(server.SOURCES):
            with self.subTest(ext=ext):
                self._stub()
                with self.assertRaises(server.ConvertError) as caught:
                    server.convert(body, ext, "html")
                self.assertEqual(422, caught.exception.status)

    def test_converted_output_is_returned(self):
        self._stub({"input.html": b"<p>converted</p>"})
        self.assertEqual(b"<p>converted</p>", server.convert(b"x", "docx", "html"))

    def test_source_is_not_read_back_as_its_own_output(self):
        """html->html returns what soffice wrote, not what was uploaded."""
        self._stub({"input.html": b"<p>normalised</p>"})
        self.assertEqual(b"<p>normalised</p>", server.convert(b"<p>raw</p>", "html", "html"))

    def test_pictures_next_to_the_document_are_inlined(self):
        self._stub(
            {
                "input.html": b'<p><img src="input_html_1.png"></p>',
                "input_html_1.png": b"\x89PNG-bytes",
            }
        )
        out = server.convert(b"x", "docx", "html").decode()
        self.assertIn("src=\"data:image/png;base64,", out)
        self.assertNotIn("input_html_1.png", out)

    def test_soffice_reads_from_a_directory_it_does_not_write_into(self):
        stub = self._stub({"input.pdf": b"%PDF-"})
        server.convert(b"x", "docx", "pdf")
        cmd = stub.calls[0]
        outdir = cmd[cmd.index("--outdir") + 1]
        self.assertNotEqual(outdir, os.path.dirname(cmd[-1]))

    def test_unsupported_formats_are_refused_before_soffice(self):
        stub = self._stub({"input.html": b"<p>x</p>"})
        with self.assertRaises(server.ConvertError) as caught:
            server.convert(b"x", "docx", "xlsx")
        self.assertEqual(400, caught.exception.status)
        with self.assertRaises(server.ConvertError) as caught:
            server.convert(b"x", "exe", "html")
        self.assertEqual(415, caught.exception.status)
        self.assertEqual([], stub.calls)


if __name__ == "__main__":
    unittest.main()
