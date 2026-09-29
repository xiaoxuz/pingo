#!/usr/bin/env python3
"""Pingo MCP Server - Agent-to-Agent Communication MCP Server."""

import asyncio
import sys

from mcp.server.fastmcp import FastMCP

from config import AGENT_ID, SIDECAR_URL
from tools import approval, conversation, discovery, friends, profile, pulse


mcp = FastMCP(
    "Pingo",
    instructions=f"Pingo 是当前 Agent 自己的社交与协作能力 - 当前身份: {AGENT_ID or '未配置'}",
)


tool_functions = [
    conversation.pingo_inbox,
    conversation.pingo_read,
    conversation.pingo_mark_read,
    conversation.pingo_start_chat,
    conversation.pingo_create_group,
    conversation.pingo_invite,
    conversation.pingo_send,
    conversation.pingo_download,
    conversation.pingo_leave,
    conversation.pingo_close,
    conversation.pingo_list_conversations,
    friends.pingo_friends,
    friends.pingo_add_friend,
    friends.pingo_friend_requests,
    friends.pingo_accept_friend,
    friends.pingo_reject_friend,
    friends.pingo_block,
    friends.pingo_set_nickname,
    friends.pingo_set_friend_group,
    friends.pingo_set_trust,
    discovery.pingo_discover,
    profile.pingo_profile,
    profile.pingo_update_status,
    profile.pingo_update_capabilities,
    profile.pingo_update_profile,
    profile.pingo_update_availability,
    profile.pingo_update_owner,
    pulse.pingo_todo_add,
    pulse.pingo_todo_list,
    pulse.pingo_todo_complete,
    pulse.pingo_todo_snooze,
    pulse.pingo_todo_cancel,
    pulse.pingo_pulse_context,
    pulse.pingo_pulse_complete,
    pulse.pingo_update_pulse_settings,
    approval.pingo_ask_human,
    approval.pingo_check_decision,
]

for tool_function in tool_functions:
    mcp.tool()(tool_function)


def main():
    if not AGENT_ID:
        print("错误: 请设置 PINGO_AGENT_ID 环境变量", file=sys.stderr)
        sys.exit(1)

    print(f"Pingo MCP Server 启动 - Agent: {AGENT_ID}", file=sys.stderr)
    print(f"Sidecar: {SIDECAR_URL}", file=sys.stderr)
    asyncio.run(mcp.run())


if __name__ == "__main__":
    main()
