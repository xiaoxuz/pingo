import asyncio
import unittest
from unittest.mock import AsyncMock, patch

from tools import conversation


class MarkReadTests(unittest.TestCase):
    def test_marks_processed_conversation_read(self):
        post = AsyncMock(return_value={"ok": True})

        with patch.object(conversation.client, "_post", post):
            result = asyncio.run(conversation.pingo_mark_read("conv-1"))

        post.assert_awaited_once_with("/conversations/conv-1/read")
        self.assertEqual(result, "会话已标记为已读")


class FileDownloadTests(unittest.TestCase):
    def test_read_exposes_file_key(self):
        get = AsyncMock(return_value=[{"from_agent": "a", "message_type": "file", "content_text": "[文件] report.pdf", "content_file": {"filename": "report.pdf", "storage_key": "files/abc.pdf", "size": 8}}])
        with patch.object(conversation.client, "_get", get):
            result = asyncio.run(conversation.pingo_read("conv-1"))
        self.assertIn("files/abc.pdf", result)

    def test_download_file_by_key(self):
        download = AsyncMock(return_value="/tmp/pingo/report.pdf")
        with patch.object(conversation.client, "_download_file", download):
            result = asyncio.run(conversation.pingo_download("files/abc.pdf", "/tmp/pingo"))
        download.assert_awaited_once_with("files/abc.pdf", "/tmp/pingo")
        self.assertIn("/tmp/pingo/report.pdf", result)


if __name__ == "__main__":
    unittest.main()
