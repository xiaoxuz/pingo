import asyncio
import unittest
from unittest.mock import AsyncMock, patch

from tools import profile


class ProfileTests(unittest.TestCase):
    def test_updates_public_profile(self):
        put = AsyncMock(return_value={"ok": True})
        with patch.object(profile.client, "_put", put):
            result = asyncio.run(profile.pingo_update_profile(
                name="尕小",
                avatar_url="https://example.test/avatar.png",
                status_text="在线串门中",
            ))
        put.assert_awaited_once_with("/me/profile", {
            "name": "尕小",
            "avatar_url": "https://example.test/avatar.png",
            "status_text": "在线串门中",
        })
        self.assertEqual(result, "公开资料已更新")

    def test_updates_availability(self):
        put = AsyncMock(return_value={"ok": True})
        with patch.object(profile.client, "_put", put):
            result = asyncio.run(profile.pingo_update_availability(
                mode="available",
                online_hours="09:00-18:00",
                timezone="Asia/Shanghai",
                max_concurrent_conversations=3,
            ))
        put.assert_awaited_once_with("/me/availability", {"availability": {
            "mode": "available",
            "online_hours": "09:00-18:00",
            "timezone": "Asia/Shanghai",
            "max_concurrent_conversations": 3,
        }})
        self.assertEqual(result, "接单偏好已更新")

    def test_updates_owner_separately(self):
        put = AsyncMock(return_value={"ok": True})
        with patch.object(profile.client, "_put", put):
            result = asyncio.run(profile.pingo_update_owner("老大", "boss@example.test"))
        put.assert_awaited_once_with("/me/owner", {
            "owner_name": "老大",
            "owner_email": "boss@example.test",
        })
        self.assertEqual(result, "归属资料已更新")


if __name__ == "__main__":
    unittest.main()
