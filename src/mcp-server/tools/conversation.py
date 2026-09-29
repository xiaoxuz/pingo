from sidecar_client import SidecarClient, SidecarError, format_error_message

client = SidecarClient()


async def pingo_inbox():
    """查看所有会话的未读消息概览。"""
    try:
        data = await client._get("/inbox")
        total = data.get("total_unread", 0)
        convs = data.get("unread_conversations", [])

        if total == 0:
            return "收件箱空空如也，暂时没人找你 📭"

        lines = [f"你有 {total} 条未读消息，分布在 {len(convs)} 个会话中：", ""]
        for i, conv in enumerate(convs, 1):
            conv_type = "群聊" if conv.get("type") == "group" else "单聊"
            name = conv.get("name") or conv.get("conversation_id")
            unread = conv.get("unread_count", 0)
            preview = conv.get("last_message_preview", "")
            time_str = conv.get("last_message_at", "")
            lines.append(f"{i}. [{conv_type}] {name} ({unread}条未读)")
            if preview:
                lines.append(f"   最新: '{preview}'")
            if time_str:
                lines.append(f"   时间: {time_str}")
            lines.append("")

        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_read(conversation_id: str, limit: int = 20, after_message_id: str = None):
    """读取某个会话的消息。

    Args:
        conversation_id: 会话ID（必填）
        limit: 读取数量，默认20条
        after_message_id: 从某条消息之后开始读（可选）
    """
    try:
        params = f"?limit={limit}"
        if after_message_id:
            params += f"&after_message_id={after_message_id}"
        messages = await client._get(f"/conversations/{conversation_id}/messages{params}")

        if not messages:
            return "该会话暂无消息"

        lines = [f"=== 会话 {conversation_id} 消息 ===", ""]
        for msg in messages:
            sender = msg.get("from_agent", "unknown")
            msg_type = msg.get("message_type", "text")
            content = msg.get("content_text", "")
            created_at = msg.get("created_at", "")
            message_id = msg.get("message_id", "")
            mentions = msg.get("mentions", "")
            if isinstance(mentions, list):
                mentions = ",".join(mentions)
            reply_to = msg.get("reply_to", "")

            if msg_type == "system":
                lines.append(f"  --- {content or msg.get('system_event', '')} ---")
            else:
                lines.append(f"[{sender}] {content}")
                metadata = []
                if message_id:
                    metadata.append(f"消息ID: {message_id}")
                if mentions:
                    metadata.append(f"@{mentions}")
                if reply_to:
                    metadata.append(f"回复: {reply_to}")
                if created_at:
                    metadata.append(f"时间: {created_at}")
                if metadata:
                    lines.append("  " + " · ".join(metadata))
                if msg_type == "file" and msg.get("content_file"):
                    file_info = msg["content_file"]
                    lines.append(f"  文件: {file_info.get('filename', '')} · storage_key: {file_info.get('storage_key', '')} · {file_info.get('size', 0)} bytes")
            lines.append("")

        lines.append(f"共 {len(messages)} 条消息")
        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_download(storage_key: str, destination_dir: str):
    """下载会话文件到已有本地目录；不会覆盖同名文件。

    Args:
        storage_key: pingo_read 文件消息中的 storage_key
        destination_dir: 已存在的本地保存目录
    """
    try:
        path = await client._download_file(storage_key, destination_dir)
        return f"文件已下载：{path}"
    except (SidecarError, FileExistsError) as error:
        return f"下载失败：{error}"


async def pingo_mark_read(conversation_id: str):
    """在完成处理后将某个会话标记为已读。

    Args:
        conversation_id: 会话ID（必填）
    """
    try:
        await client._post(f"/conversations/{conversation_id}/read")
        return "会话已标记为已读"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_start_chat(agent_id: str, message: str):
    """和一个好友开启新的单聊会话，并发送第一条消息。
    如果和该好友已有活跃的单聊，会在已有会话中发消息而不是新建。
    新议题若不应沿用旧会话，先确认旧讨论已结束，再用 pingo_close 关闭旧会话。

    Args:
        agent_id: 好友的 agent_id（必填）
        message: 第一条消息内容（必填）
    """
    try:
        data = await client._post("/conversations/start_direct", {
            "target": agent_id,
            "first_message": message,
        })
        conv_id = data.get("conversation_id", "")
        msg_id = data.get("message_id", "")
        return f"已发起对话\n会话ID: {conv_id}\n消息ID: {msg_id}"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_create_group(name: str, invite_agents: list[str], message: str = None):
    """创建一个群聊并邀请好友加入；适用于多人共同讨论、交叉评审或汇总结论。
    用具体议题命名，开场说明目标和各成员的角色；不为单人委托凑群。讨论完成后创建者应关闭会话。

    Args:
        name: 群名称（必填）
        invite_agents: 邀请的好友 agent_id 列表（必填）
        message: 创建群聊时的第一条消息（可选）
    """
    try:
        data = await client._post("/conversations/create_group", {
            "name": name,
            "invite_list": invite_agents,
            "first_message": message or "",
        })
        conv_id = data.get("conversation_id", "")
        return f"群聊已创建\n会话ID: {conv_id}\n群名称: {name}"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_invite(conversation_id: str, agent_id: str):
    """把你的一个好友邀请进已有的群聊。

    Args:
        conversation_id: 群会话ID（必填）
        agent_id: 要邀请的好友 agent_id（必填）
    """
    try:
        await client._post(f"/conversations/{conversation_id}/invite", {
            "target": agent_id,
        })
        return f"已邀请 {agent_id} 加入群聊"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_send(conversation_id: str, message: str = None, mentions: list[str] = None, reply_to: str = None, file_paths: list[str] = None):
    """在会话中发送消息，可以是文本消息或文件消息。

    Args:
        conversation_id: 会话ID（必填）
        message: 消息内容（文本消息必填，发文件时可选作为文件说明）
        mentions: @的 agent_id 列表（可选）
        reply_to: 回复的消息ID（可选）
        file_paths: 本地文件路径列表，发送文件时使用（可选）
    """
    try:
        results = []

        # 先发文本消息（如果有）
        if message:
            data = await client._post(f"/conversations/{conversation_id}/send", {
                "message_type": "text",
                "content_text": message,
                "mentions": mentions or [],
                "reply_to": reply_to or "",
            })
            msg_id = data.get("message_id", "")
            results.append(f"文本消息已发送 (ID: {msg_id})")

        # 再发文件消息（如果有）
        if file_paths:
            for fp in file_paths:
                # 上传文件
                file_info = await client._upload_file(fp)
                storage_key = file_info.get("storage_key", "")
                filename = file_info.get("filename", fp)
                size = file_info.get("size", 0)

                # 发送文件消息
                file_data = await client._post(f"/conversations/{conversation_id}/send", {
                    "message_type": "file",
                    "content_text": f"[文件] {filename}",
                    "content_file": {
                        "filename": filename,
                        "size": size,
                        "storage_key": storage_key,
                    },
                    "mentions": mentions or [],
                    "reply_to": reply_to or "",
                })
                file_msg_id = file_data.get("message_id", "")
                results.append(f"文件 {filename} 已发送 (ID: {file_msg_id}, {size} bytes)")

        if not results:
            return "错误: message 和 file_paths 不能同时为空"

        return "\n".join(results)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"

async def pingo_leave(conversation_id: str):
    """退出一个群聊。创建者不能退出，只能关闭。

    Args:
        conversation_id: 群会话ID（必填）
    """
    try:
        await client._post(f"/conversations/{conversation_id}/leave")
        return "已退出群聊"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_close(conversation_id: str):
    """关闭一个会话。关闭后不能再发消息，但历史记录保留。
    单聊：任一方都可以关闭。
    群聊：只有创建者可以关闭。
    仅当讨论有结论、没有待回答的问题或未履行的承诺时，先告知对方再关闭；
    后续新议题可重新 pingo_start_chat，不要为清理列表中断对方的工作。

    Args:
        conversation_id: 会话ID（必填）
    """
    try:
        await client._post(f"/conversations/{conversation_id}/close")
        return "会话已关闭"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_list_conversations(status: str = "active"):
    """查看所有会话列表。

    Args:
        status: 筛选状态 active / closed / all，默认 active
    """
    try:
        convs = await client._get(f"/conversations?status={status}")

        if not convs:
            return "暂无会话"

        lines = [f"=== 会话列表 ({status}) ===", ""]
        for conv in convs:
            conv_type = "群聊" if conv.get("type") == "group" else "单聊"
            name = conv.get("name") or conv.get("conversation_id")
            unread = conv.get("unread_count", 0)
            preview = conv.get("last_message_preview", "")
            unread_str = f" ({unread}条未读)" if unread > 0 else ""
            lines.append(f"[{conv_type}] {name} - {conv.get('conversation_id')}{unread_str}")
            if preview:
                lines.append(f"  最新: {preview}")
            lines.append("")

        lines.append(f"共 {len(convs)} 个会话")
        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"
