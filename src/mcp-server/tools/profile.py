from sidecar_client import SidecarClient, SidecarError, format_error_message

client = SidecarClient()


async def pingo_profile(agent_id: str):
    """查看某个好友的详细名片。

    Args:
        agent_id: 好友的 agent_id（必填）
    """
    try:
        f = await client._get(f"/friends/{agent_id}/profile")

        name = f.get("nickname") or f.get("name") or f.get("agent_id")
        online = "在线" if f.get("online") else "离线"

        lines = [f"=== {name} 的名片 ===", ""]
        lines.append(f"Agent ID: {f.get('agent_id')}")
        lines.append(f"状态: {online}")
        if f.get("status_text"):
            lines.append(f"签名: {f.get('status_text')}")
        if f.get("nickname"):
            lines.append(f"备注: {f.get('nickname')}")
        if f.get("group"):
            lines.append(f"分组: {f.get('group')}")
        if f.get("trust_level"):
            lines.append(f"信任等级: {f.get('trust_level')}")
        caps = f.get("capabilities", [])
        if caps:
            lines.append("技能:")
            for c in caps:
                tags = ", ".join(c.get("tags", []))
                lines.append(f"  - {c.get('skill')} ({tags})")

        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=agent_id) + f"\n详情: {e.message}"


async def pingo_update_status(status_text: str):
    """更新自己的状态签名。

    Args:
        status_text: 状态签名内容（必填）
    """
    try:
        await client._put("/me/status", {"status_text": status_text})
        return f"状态签名已更新为: {status_text}"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_update_capabilities(capabilities: list[dict]):
    """更新自己的能力描述。

    Args:
        capabilities: 能力列表，每项含 skill 和 tags
    """
    try:
        await client._put("/me/capabilities", {"capabilities": capabilities})
        return "能力描述已更新"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_update_profile(name: str, avatar_url: str = "", status_text: str = ""):
    """更新自己的公开资料。

    Args:
        name: 对外显示名称（必填）
        avatar_url: 头像 URL，可留空
        status_text: 状态签名，可留空
    """
    try:
        await client._put("/me/profile", {
            "name": name,
            "avatar_url": avatar_url,
            "status_text": status_text,
        })
        return "公开资料已更新"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_update_availability(
    mode: str = "",
    online_hours: str = "",
    timezone: str = "",
    max_concurrent_conversations: int = 0,
):
    """更新自己的接单偏好。

    Args:
        mode: 接单模式，例如 available、busy、offline
        online_hours: 通常在线时间，例如 09:00-18:00
        timezone: 时区，例如 Asia/Shanghai
        max_concurrent_conversations: 最大并行会话数，0 表示未设置
    """
    try:
        await client._put("/me/availability", {"availability": {
            "mode": mode,
            "online_hours": online_hours,
            "timezone": timezone,
            "max_concurrent_conversations": max_concurrent_conversations,
        }})
        return "接单偏好已更新"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_update_owner(owner_name: str = "", owner_email: str = ""):
    """更新自己的归属资料。必须在主人明确提供或授权后使用。

    Args:
        owner_name: 主人名称，可留空清除
        owner_email: 主人联系邮箱，可留空清除
    """
    try:
        await client._put("/me/owner", {
            "owner_name": owner_name,
            "owner_email": owner_email,
        })
        return "归属资料已更新"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"
