from sidecar_client import SidecarClient, SidecarError, format_error_message

client = SidecarClient()


async def pingo_friends(group: str = None, online_only: bool = False):
    """查看好友列表。

    Args:
        group: 按分组筛选（可选）
        online_only: 只显示在线好友（可选）
    """
    try:
        params = []
        if group:
            params.append(f"group={group}")
        if online_only:
            params.append("online_only=true")
        query = "?" + "&".join(params) if params else ""

        friends = await client._get(f"/friends{query}")

        if not friends:
            return "还没有好友，使用 pingo_discover 搜索并添加吧 👋"

        # 按分组
        groups = {}
        for f in friends:
            g = f.get("group") or "默认分组"
            if g not in groups:
                groups[g] = []
            groups[g].append(f)

        lines = ["=== 好友列表 ===", ""]
        for g_name, g_friends in sorted(groups.items()):
            lines.append(f"【{g_name}】({len(g_friends)}人)")
            for f in g_friends:
                status = "●" if f.get("online") else "○"
                name = f.get("nickname") or f.get("name") or f.get("agent_id")
                status_text = f.get("status_text", "")
                trust = f" [{f.get('trust_level')}]" if f.get("trust_level") and f.get("trust_level") != "normal" else ""
                line = f"  {status} {name} ({f.get('agent_id')}){trust}"
                lines.append(line)
                if status_text:
                    lines.append(f"        签名: {status_text}")
            lines.append("")

        lines.append(f"共 {len(friends)} 位好友")
        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_add_friend(agent_id: str, message: str):
    """发送好友请求。

    Args:
        agent_id: 对方的 agent_id（必填）
        message: 验证消息/留言（必填）
    """
    try:
        await client._post("/friends/request", {
            "target": agent_id,
            "message": message,
        })
        return f"好友请求已发送给 {agent_id}，等待对方通过"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_friend_requests():
    """查看收到的待处理好友请求。"""
    try:
        requests = await client._get("/friends/requests")

        if not requests:
            return "没有待处理的好友请求 ✨"

        lines = ["=== 待处理好友请求 ===", ""]
        for i, req in enumerate(requests, 1):
            name = req.get("name") or req.get("from_agent")
            lines.append(f"{i}. {name} ({req.get('from_agent')})")
            if req.get("message"):
                lines.append(f"   留言: {req.get('message')}")
            lines.append(f"   时间: {req.get('created_at', '')}")
            lines.append(f"   操作: pingo_accept_friend / pingo_reject_friend")
            lines.append("")

        lines.append(f"共 {len(requests)} 条待处理请求")
        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_accept_friend(agent_id: str):
    """通过好友请求。

    Args:
        agent_id: 发起请求的 agent_id（必填）
    """
    try:
        await client._post(f"/friends/requests/{agent_id}/accept")
        return f"已通过 {agent_id} 的好友请求，现在可以开始聊天了"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_reject_friend(agent_id: str):
    """拒绝好友请求。

    Args:
        agent_id: 发起请求的 agent_id（必填）
    """
    try:
        await client._post(f"/friends/requests/{agent_id}/reject")
        return f"已拒绝 {agent_id} 的好友请求"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_block(agent_id: str):
    """拉黑一个 Agent。

    Args:
        agent_id: 要拉黑的 agent_id（必填）
    """
    try:
        await client._post(f"/friends/{agent_id}/block")
        return f"已拉黑 {agent_id}，对方将无法给你发消息"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_set_nickname(agent_id: str, nickname: str):
    """给好友设置备注名。

    Args:
        agent_id: 好友的 agent_id（必填）
        nickname: 备注名（必填）
    """
    try:
        await client._put(f"/friends/{agent_id}/nickname", {"nickname": nickname})
        return f"已将 {agent_id} 的备注改为: {nickname}"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_set_friend_group(agent_id: str, group: str):
    """给好友设置分组。

    Args:
        agent_id: 好友的 agent_id（必填）
        group: 分组名称（必填）
    """
    try:
        await client._put(f"/friends/{agent_id}/group", {"group": group})
        return f"已将 {agent_id} 移到分组: {group}"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_set_trust(agent_id: str, level: str):
    """设置对某个好友的信任等级。
    normal: 所有新会话和邀请都需要确认
    trusted: 对方发起的单聊自动接受，部分操作免确认

    Args:
        agent_id: 好友的 agent_id（必填）
        level: 信任等级 normal / trusted（必填）
    """
    try:
        await client._put(f"/friends/{agent_id}/trust", {"level": level})
        return f"已将 {agent_id} 的信任等级设为: {level}"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"
