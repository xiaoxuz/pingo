import json
from sidecar_client import SidecarClient, SidecarError, format_error_message

client = SidecarClient()


async def pingo_ask_human(question: str, options: list[dict], context: str = None, urgency: str = "normal"):
    """当你不确定某个决策时，请求背后的人来判断。
    调用后会弹通知给人，然后你需要用 pingo_check_decision 轮询结果。

    Args:
        question: 问题描述（必填）
        options: 选项列表，每项含 value、label、description
        context: 补充背景信息（可选）
        urgency: 紧急程度 low / normal / high（可选）
    """
    try:
        data = await client._post("/approvals/request", {
            "question": question,
            "options": options,
            "context": context or "",
            "urgency": urgency,
        })
        approval_id = data.get("approval_id", "")
        return f"已请求人工决策\n审批ID: {approval_id}\n使用 pingo_check_decision('{approval_id}') 查看结果"
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"


async def pingo_check_decision(approval_id: str):
    """查询人工决策的结果。

    Args:
        approval_id: 审批ID（必填）

    Returns:
        status: pending / decided / timeout
        decision: 具体选项值
        comment: 人写的备注
    """
    try:
        data = await client._get(f"/approvals/{approval_id}")
        status = data.get("status", "pending")
        decision = data.get("decision", "")
        comment = data.get("comment", "")

        lines = [f"=== 决策状态: {status} ===", ""]
        if status == "decided":
            lines.append(f"决策结果: {decision}")
            if comment:
                lines.append(f"备注: {comment}")
            lines.append(f"决定时间: {data.get('decided_at', '')}")
        elif status == "timeout":
            lines.append(f"超时，自动结果: {decision}")
        else:
            lines.append("等待决策中...")
            lines.append(f"创建时间: {data.get('created_at', '')}")
            lines.append(f"超时时间: {data.get('timeout_at', '')}")

        return "\n".join(lines)
    except SidecarError as e:
        return format_error_message(e.code, agent_id=client.agent_id) + f"\n详情: {e.message}"
