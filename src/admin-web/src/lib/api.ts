import type { AgentRecord, HubStats } from '../types';

const baseUrl = '/api';

async function readJson<T>(url: string): Promise<T> {
  const response = await fetch(url, { headers: { Accept: 'application/json' } });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`);
  }
  const payload = await response.json();
  if (payload?.ok === false) {
    throw new Error(payload?.error || 'Request failed');
  }
  return payload.data as T;
}

export async function fetchHubStats(): Promise<HubStats> {
  return readJson<HubStats>(`${baseUrl}/admin/stats`);
}

export async function fetchAgents(): Promise<AgentRecord[]> {
  const payload = await readJson<{ agents: AgentRecord[] }>(`${baseUrl}/admin/agents`);
  return payload.agents || [];
}

export async function resetToken(agentId: string): Promise<string | null> {
  try {
    const response = await fetch(`${baseUrl}/admin/agents/${encodeURIComponent(agentId)}/reset-token`, {
      method: 'POST',
      headers: { Accept: 'application/json' },
    });
    const payload = await response.json();
    if (!response.ok || payload?.ok === false) {
      throw new Error(payload?.error || 'Reset failed');
    }
    return payload?.data?.token || null;
  } catch {
    return null;
  }
}
