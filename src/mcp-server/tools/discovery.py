from sidecar_client import SidecarClient, SidecarError, format_error_message

client = SidecarClient()


async def pingo_discover(keyword: str = None, skill: str = None, tags: list[str] = None, online_only: bool = False):
    """全局搜索Agent。只能看到公开信息。要交互需先加好友。

    Args:
        keyword: 关键词搜索（可选）
        skill: 按技能搜索（可选）
        tags: 按标签筛选（可选）
        online_only: 只显示在线的（可选）
    """
    try:
        data = await client._post("/discover", {
            "keyword": keyword or "",
            "skill": skill or "",
            "tags": tags or [],
            "online_only": online_only,
            "limit": 20,
        })

        agents = data.get("agents", [])
        total = data.get("total", 0)

        if not agents:
            return "没有找到匹配的 Agent"

        lines = [f"=== 搜索结果 (共{total}个) ===", ""]
        for i, a in enumerate(agents, 1):
            status = "●" if a.get("online") else "○"
            friend_status = " [已是好友]" if a.get("is_friend") else " [非好友]"
            name = a.get("name") or a.get("agent_id")
            lines.append(f"{i}. {status} {name} ({a.get('agent_id')}){friend_status}")
            if a.get("status_text"):
                lines.append(f"   签名: {a.get('status_text')}")
            caps = a.get("capabilities", [])
            if caps:
                cap_strs = [f"{c.get('skill')}({', '.join(c.get('tags', []))})" for c in caps]
                lines.append(f"   技能: {'; '.join(cap_strs)}")
            lines.append("")

        lines.append("提示: 使用 pingo_add_friend 添加好友")
        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"
