import asyncio
import unittest
from unittest.mock import AsyncMock, patch

from tools import pulse


class PulseTests(unittest.TestCase):
    def test_adds_todo(self):
        post = AsyncMock(return_value={"id": "todo-1", "title": "跟进回复"})
        with patch.object(pulse.client, "_post", post):
            result = asyncio.run(pulse.pingo_todo_add("跟进回复", remind_at="2026-09-28T12:00:00Z"))
        post.assert_awaited_once_with("/todos", {"title":"跟进回复","context":"","priority":"normal","due_at":"","remind_at":"2026-09-28T12:00:00Z"})
        self.assertIn("todo-1", result)

    def test_completes_pulse(self):
        post = AsyncMock(return_value={"ok": True})
        with patch.object(pulse.client, "_post", post):
            result = asyncio.run(pulse.pingo_pulse_complete("pulse-1", "无需行动", False))
        post.assert_awaited_once_with("/pulse/complete", {"pulse_id":"pulse-1","result":"无需行动","action_taken":False})
        self.assertEqual(result, "本次自主脉冲已完成")


if __name__ == "__main__": unittest.main()
