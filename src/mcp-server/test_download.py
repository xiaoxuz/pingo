import asyncio
import tempfile
import unittest
from pathlib import Path
from unittest.mock import AsyncMock, patch

import httpx

from sidecar_client import SidecarClient, SidecarError


class DownloadTests(unittest.TestCase):
    def test_download_saves_file_in_existing_directory(self):
        client = SidecarClient()
        client.agent_id = "agt-test"
        with tempfile.TemporaryDirectory() as directory:
            response = httpx.Response(200, content=b"hello", headers={"Content-Disposition": 'attachment; filename="report.txt"'}, request=httpx.Request("GET", "http://localhost/files/abc"))
            fake = AsyncMock()
            fake.get.return_value = response
            with patch("sidecar_client.httpx.AsyncClient") as transport:
                transport.return_value.__aenter__.return_value = fake
                saved = asyncio.run(client._download_file("files/abc", directory))
            self.assertEqual(Path(saved).read_bytes(), b"hello")
            self.assertEqual(fake.get.call_args.kwargs["headers"]["X-Agent-ID"], "agt-test")

    def test_download_uses_agent_identity_and_does_not_overwrite(self):
        client = SidecarClient()
        client.agent_id = "agt-test"
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory) / "report.txt"
            destination.write_text("original")
            response = httpx.Response(200, content=b"new data", headers={"Content-Disposition": 'attachment; filename="report.txt"'}, request=httpx.Request("GET", "http://localhost/files/abc"))
            fake = AsyncMock()
            fake.get.return_value = response
            with patch("sidecar_client.httpx.AsyncClient") as transport:
                transport.return_value.__aenter__.return_value = fake
                with self.assertRaises(FileExistsError):
                    asyncio.run(client._download_file("files/abc", directory))
            self.assertEqual(destination.read_text(), "original")
            fake.get.assert_awaited_once()
            self.assertEqual(fake.get.call_args.kwargs["headers"]["X-Agent-ID"], "agt-test")

    def test_download_rejects_bad_key(self):
        with self.assertRaises(SidecarError):
            asyncio.run(SidecarClient()._download_file("../secrets", "/tmp"))
