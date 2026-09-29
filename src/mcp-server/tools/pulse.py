from sidecar_client import SidecarClient, SidecarError, format_error_message

client = SidecarClient()

def _error(error): return format_error_message(error.code, agent_id=client.agent_id) + f"\n详情: {error.message}"

async def pingo_todo_add(title: str, context: str = "", priority: str = "normal", due_at: str = "", remind_at: str = ""):
    """给自己添加需要持续关注的待办。时间使用 RFC3339 格式。"""
    try:
        item = await client._post("/todos", {"title":title,"context":context,"priority":priority,"due_at":due_at,"remind_at":remind_at})
        return f"待办已创建：{item.get('title')}（{item.get('id')}）"
    except SidecarError as error: return _error(error)

async def pingo_todo_list(status: str = "pending"):
    """查看自己的待办；status 可为 pending、completed、cancelled 或空字符串。"""
    try:
        items = await client._get(f"/todos?status={status}")
        if not items: return "没有符合条件的待办"
        return "\n".join(f"- {item.get('id')} · {item.get('title')} · {item.get('priority')} · 提醒 {item.get('RemindAt') or item.get('remind_at') or '未设置'}" for item in items)
    except SidecarError as error: return _error(error)

async def pingo_todo_complete(todo_id: str):
    """完成自己的一个待办。"""
    try: await client._post(f"/todos/{todo_id}/complete"); return "待办已完成"
    except SidecarError as error: return _error(error)

async def pingo_todo_snooze(todo_id: str, remind_at: str):
    """把待办延后到指定 RFC3339 时间。"""
    try: await client._post(f"/todos/{todo_id}/snooze", {"remind_at":remind_at}); return "待办提醒时间已更新"
    except SidecarError as error: return _error(error)

async def pingo_todo_cancel(todo_id: str):
    """取消自己的一个待办。"""
    try: await client._delete(f"/todos/{todo_id}"); return "待办已取消"
    except SidecarError as error: return _error(error)

async def pingo_pulse_context():
    """读取本次自主脉冲需要的待办、配置和最近历史。"""
    try: return await client._get("/pulse/context")
    except SidecarError as error: return _error(error)

async def pingo_pulse_complete(pulse_id: str, result: str, action_taken: bool):
    """完成当前自主脉冲并记录自己的判断和是否采取行动。"""
    try:
        await client._post("/pulse/complete", {"pulse_id":pulse_id,"result":result,"action_taken":action_taken})
        return "本次自主脉冲已完成"
    except SidecarError as error: return _error(error)

async def pingo_update_pulse_settings(mode: str, quiet_start: str = "23:00", quiet_end: str = "08:00", cooldown_minutes: int = 120, daily_budget: int = 1):
    """调整自己的自主等级、安静时间、冷却和每日自主探索额度。"""
    try:
        await client._put("/pulse/settings", {"mode":mode,"quiet_start":quiet_start,"quiet_end":quiet_end,"cooldown_minutes":cooldown_minutes,"daily_budget":daily_budget})
        return "自主脉冲设置已更新"
    except SidecarError as error: return _error(error)
