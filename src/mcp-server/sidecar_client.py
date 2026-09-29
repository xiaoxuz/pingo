import httpx
import os
from pathlib import Path
from urllib.parse import quote
from config import SIDECAR_URL, AGENT_ID


class SidecarError(Exception):
    def __init__(self, code, message):
        self.code = code
        self.message = message
        super().__init__(message)


class SidecarClient:
    def __init__(self):
        self.base_url = SIDECAR_URL.rstrip("/")
        self.agent_id = AGENT_ID
        self.timeout = 10.0

    def _headers(self):
        return {
            "X-Agent-ID": self.agent_id,
            "Content-Type": "application/json",
        }

    async def _get(self, path):
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.get(
                    f"{self.base_url}{path}",
                    headers=self._headers(),
                )
                return self._parse_response(resp)
        except httpx.ConnectError:
            raise SidecarError("CONNECTION_FAILED",
                "Pingo 系统未连接。请确认 Sidecar 是否在运行。")
        except httpx.TimeoutException:
            raise SidecarError("TIMEOUT",
                "请求超时，Sidecar 可能繁忙，稍后再试。")

    async def _post(self, path, data=None):
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.post(
                    f"{self.base_url}{path}",
                    headers=self._headers(),
                    json=data or {},
                )
                return self._parse_response(resp)
        except httpx.ConnectError:
            raise SidecarError("CONNECTION_FAILED",
                "Pingo 系统未连接。请确认 Sidecar 是否在运行。")
        except httpx.TimeoutException:
            raise SidecarError("TIMEOUT",
                "请求超时，Sidecar 可能繁忙，稍后再试。")

    async def _put(self, path, data=None):
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.put(
                    f"{self.base_url}{path}",
                    headers=self._headers(),
                    json=data or {},
                )
                return self._parse_response(resp)
        except httpx.ConnectError:
            raise SidecarError("CONNECTION_FAILED",
                "Pingo 系统未连接。请确认 Sidecar 是否在运行。")
        except httpx.TimeoutException:
            raise SidecarError("TIMEOUT",
                "请求超时，Sidecar 可能繁忙，稍后再试。")

    async def _delete(self, path):
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.delete(
                    f"{self.base_url}{path}",
                    headers=self._headers(),
                )
                return self._parse_response(resp)
        except httpx.ConnectError:
            raise SidecarError("CONNECTION_FAILED",
                "Pingo 系统未连接。请确认 Sidecar 是否在运行。")
        except httpx.TimeoutException:
            raise SidecarError("TIMEOUT",
                "请求超时，Sidecar 可能繁忙，稍后再试。")

    def _parse_response(self, resp):
        try:
            data = resp.json()
        except Exception:
            raise SidecarError("INVALID_RESPONSE", "无效的响应格式")

        if not data.get("ok", False):
            code = data.get("code", "UNKNOWN")
            message = data.get("error", "未知错误")
            raise SidecarError(code, message)

        return data.get("data")

    async def _upload_file(self, file_path: str):
        """上传文件到 Sidecar，返回文件信息。"""
        import os
        if not os.path.exists(file_path):
            raise SidecarError("FILE_NOT_FOUND", f"文件不存在: {file_path}")

        filename = os.path.basename(file_path)
        try:
            async with httpx.AsyncClient(timeout=60.0) as client:
                with open(file_path, 'rb') as f:
                    files = {'file': (filename, f)}
                    headers = {'X-Agent-ID': self.agent_id}
                    resp = await client.post(
                        f"{self.base_url}/files/upload",
                        headers=headers,
                        files=files,
                    )
                return self._parse_response(resp)
        except httpx.ConnectError:
            raise SidecarError("CONNECTION_FAILED",
                "Pingo 系统未连接。请确认 Sidecar 是否在运行。")
        except httpx.TimeoutException:
            raise SidecarError("TIMEOUT",
                "文件上传超时，文件可能太大，请稍后再试。")

    async def _download_file(self, storage_key: str, destination_dir: str):
        if not storage_key.startswith("files/") or not storage_key[6:] or "/" in storage_key[6:] or storage_key[6:] in (".", ".."):
            raise SidecarError("INVALID_FILE_KEY", "无效的文件 key")
        directory = Path(destination_dir).expanduser().resolve()
        if not directory.is_dir():
            raise SidecarError("INVALID_DESTINATION", "目标目录不存在")
        try:
            async with httpx.AsyncClient(timeout=120.0) as client:
                response = await client.get(
                    f"{self.base_url}/files/{quote(storage_key[6:], safe='')}",
                    headers={"X-Agent-ID": self.agent_id},
                )
            if response.status_code != 200:
                raise SidecarError("DOWNLOAD_FAILED", f"下载失败: HTTP {response.status_code}")
            from email.message import Message
            disposition = Message()
            disposition["Content-Disposition"] = response.headers.get("Content-Disposition", "")
            filename = disposition.get_filename() or storage_key[6:]
            filename = os.path.basename(filename.replace("\\", "/"))
            if filename in ("", ".", ".."):
                raise SidecarError("INVALID_FILENAME", "文件名无效")
            path = directory / filename
            with path.open("xb") as output:
                output.write(response.content)
            return str(path)
        except (httpx.ConnectError, httpx.TimeoutException) as error:
            raise SidecarError("DOWNLOAD_FAILED", "Sidecar 文件下载失败") from error



ERROR_MESSAGES = {
    "NOT_FRIENDS": "你和 {agent_id} 还不是好友，无法发起对话。\n要先加好友吗？使用 pingo_add_friend 发送好友请求。",
    "CONV_CLOSED": "该会话已关闭，无法再发消息。如需继续沟通，请开启新会话。",
    "NOT_CREATOR": "只有群创建者可以关闭群聊。你可以选择退出 (pingo_leave)。",
    "BLOCKED": "无法向该用户发送消息。",
    "ALREADY_FRIENDS": "你和 {agent_id} 已经是好友了，可以直接开始聊天。",
    "AGENT_ALREADY_CONNECTED": "该 Agent 已在其他机器上在线。同一个 Agent 不能同时两处登录。\n请先在另一台机器上移除该 Agent 配置。",
    "RATE_LIMITED": "发送过于频繁，请稍后再试。",
    "AGENT_NOT_FOUND": "找不到 agent: {agent_id}，请检查 ID 是否正确。",
}


def format_error_message(code, **kwargs):
    if code in ERROR_MESSAGES:
        return ERROR_MESSAGES[code].format(**kwargs)
    return f"操作失败: {code}"
