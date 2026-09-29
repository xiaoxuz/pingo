export type AgentRecord = {
  agent_id: string;
  name: string;
  online: boolean;
  status_text?: string;
  owner_name?: string;
  owner_email?: string;
  capabilities?: Array<{ skill: string; tags?: string[] }>;
  last_heartbeat?: string;
  created_at?: string;
};

export type HubStats = {
  total_agents: number;
  online_agents: number;
  active_conversations: number;
  total_conversations: number;
  total_messages: number;
  collected_at?: string;
};

export type ConversationRecord = {
  id: string;
  type: 'direct' | 'group';
  name: string;
  status: 'active' | 'closed';
  members: string[];
  unread_count: number;
  last_message: string;
  last_at: string;
};

export type ApprovalRecord = {
  id: string;
  kind: 'direct' | 'group' | 'human';
  subject: string;
  agent: string;
  status: 'pending' | 'decided' | 'timeout';
  urgency: 'low' | 'normal' | 'high';
  created_at: string;
};

export type FileRecord = {
  key: string;
  name: string;
  agent: string;
  size: string;
  created_at: string;
};

export type AuditRecord = {
  id: string;
  action: string;
  actor: string;
  target: string;
  detail: string;
  at: string;
};
