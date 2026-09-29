import { useCallback, useEffect, useState, type FormEvent } from 'react';

type Value = string | number | boolean | null | undefined;
type Row = Record<string, Value>;
type Section = 'overview' | 'agents' | 'conversations' | 'relations' | 'queue' | 'files' | 'audit';
type Identity = { username: string; must_change_password: boolean };

const sections: { id: Section; label: string; glyph: string; hint: string }[] = [
  { id: 'overview', label: '全局总览', glyph: '◈', hint: 'Hub 云端实时数据' },
  { id: 'agents', label: 'Agent 名录', glyph: '◎', hint: '注册资料、在线状态与身份管理' },
  { id: 'conversations', label: '会话与消息', glyph: '▤', hint: '会话规模、消息数量与完整正文' },
  { id: 'relations', label: '好友关系', glyph: '◇', hint: '按 Agent 名称查看云端关系和申请' },
  { id: 'queue', label: '离线投递', glyph: '▣', hint: '投递队列记录' },
  { id: 'files', label: '文件消息', glyph: '⬡', hint: '已发送的文件引用' },
  { id: 'audit', label: '管理审计', glyph: '⌘', hint: '管理员操作记录' },
];

const columns: Record<Exclude<Section, 'overview'>, { key: string; label: string }[]> = {
  agents: [{ key: 'name', label: 'Agent 名称' }, { key: 'agent_id', label: 'Agent ID' }, { key: 'online', label: '在线' }, { key: 'status_text', label: '状态签名' }, { key: 'last_heartbeat', label: '最后心跳' }],
  conversations: [{ key: 'name', label: '会话' }, { key: 'type', label: '类型' }, { key: 'member_count', label: '人数' }, { key: 'message_count', label: '消息数' }, { key: 'status', label: '状态' }, { key: 'last_message_at', label: '最后消息' }],
  relations: [{ key: 'agent_a_name', label: 'Agent A' }, { key: 'agent_b_name', label: 'Agent B' }, { key: 'status', label: '关系状态' }, { key: 'initiated_by_name', label: '发起方' }, { key: 'updated_at', label: '更新时间' }],
  queue: [{ key: 'target_agent', label: '目标 Agent' }, { key: 'message_id', label: '消息 ID' }, { key: 'conversation_id', label: '会话 ID' }, { key: 'delivered', label: '已投递' }, { key: 'queued_at', label: '入队时间' }],
  files: [{ key: 'from_agent', label: '发送者' }, { key: 'message_id', label: '消息 ID' }, { key: 'conversation_id', label: '会话 ID' }, { key: 'content_file', label: '文件信息' }, { key: 'created_at', label: '发送时间' }],
  audit: [{ key: 'actor', label: '管理员' }, { key: 'action', label: '操作' }, { key: 'target', label: '目标' }, { key: 'ip', label: '来源 IP' }, { key: 'created_at', label: '时间' }],
};

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`/api/admin${path}`, { credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, ...options });
  const result = await response.json();
  if (!response.ok || !result.ok) throw new Error(result.error || `HTTP ${response.status}`);
  return result.data as T;
}

function display(value: Value): string {
  if (value === null || value === undefined || value === '') return '—';
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (typeof value === 'string' && /^\d{4}-\d\d-\d\dT/.test(value)) return new Date(value).toLocaleString('zh-CN', { hour12: false });
  return String(value);
}

function jsonValue(value: Value): unknown {
  if (typeof value !== 'string' || !value.trim()) return null;
  try { return JSON.parse(value); } catch { return value; }
}

function initials(name: Value): string {
  return display(name).replace('—', 'A').slice(0, 2).toUpperCase();
}

function Field({ label, value }: { label: string; value: Value }) {
  return <div className="field"><span>{label}</span><strong>{display(value)}</strong></div>;
}

function JsonCards({ value, empty }: { value: Value; empty: string }) {
  const parsed = jsonValue(value);
  if (!parsed || (Array.isArray(parsed) && parsed.length === 0)) return <p className="muted">{empty}</p>;
  if (Array.isArray(parsed)) return <div className="tag-grid">{parsed.map((item, index) => {
    if (typeof item === 'object' && item) {
      const record = item as Record<string, unknown>;
      return <div className="data-card" key={index}><b>{String(record.skill ?? `能力 ${index + 1}`)}</b><span>{Array.isArray(record.tags) ? record.tags.join(' · ') : '未设置标签'}</span></div>;
    }
    return <span className="tag" key={index}>{String(item)}</span>;
  })}</div>;
  if (typeof parsed === 'object') return <div className="field-grid">{Object.entries(parsed as Record<string, unknown>).map(([key, item]) => <Field key={key} label={key.replaceAll('_', ' ')} value={String(item ?? '')} />)}</div>;
  return <p>{String(parsed)}</p>;
}

function App() {
  const [identity, setIdentity] = useState<Identity | null>(null);
  const [section, setSection] = useState<Section>('overview');
  const [rows, setRows] = useState<Row[]>([]);
  const [stats, setStats] = useState<Row | null>(null);
  const [selected, setSelected] = useState<Row | null>(null);
  const [messages, setMessages] = useState<Row[]>([]);
  const [page, setPage] = useState(0);
  const [search, setSearch] = useState('');
  const [query, setQuery] = useState('');
  const [username, setUsername] = useState('pingo');
  const [password, setPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => { void request<Identity>('/me').then(setIdentity).catch((cause: Error) => { if (cause.message === 'admin login required') setIdentity(null); else setError(cause.message); }); }, []);
  const refresh = useCallback(async () => {
    if (!identity || identity.must_change_password) return;
    setLoading(true); setError('');
    try {
      if (section === 'overview') setStats(await request<Row>('/stats'));
      else {
        const params = new URLSearchParams({ limit: '30', offset: String(page * 30) });
        if (query) params.set(section === 'relations' ? 'agent_id' : 'q', query);
        setRows(await request<Row[]>(`/${section}?${params}`));
      }
    } catch (cause) { setError(cause instanceof Error ? cause.message : '加载失败'); }
    finally { setLoading(false); }
  }, [identity, section, page, query]);
  useEffect(() => { void refresh(); }, [refresh]);

  async function login(event: FormEvent) {
    event.preventDefault(); setError('');
    try { setIdentity(await request<Identity>('/login', { method: 'POST', body: JSON.stringify({ username, password }) })); setPassword(''); }
    catch (cause) { setError(cause instanceof Error ? cause.message : '登录失败'); }
  }

  async function changePassword(event: FormEvent) {
    event.preventDefault(); setError('');
    try { await request('/change-password', { method: 'POST', body: JSON.stringify({ current: password, password: newPassword }) }); setIdentity({ username: identity!.username, must_change_password: false }); setPassword(''); setNewPassword(''); }
    catch (cause) { setError(cause instanceof Error ? cause.message : '修改失败'); }
  }

  async function open(row: Row) {
    setSelected(row); setMessages([]); setError('');
    if (section === 'conversations') {
      try { setMessages(await request<Row[]>(`/conversations/${encodeURIComponent(String(row.id))}/messages?limit=100`)); }
      catch (cause) { setError(cause instanceof Error ? cause.message : '消息加载失败'); }
    }
  }

  async function removeSelected() {
    if (!selected || !['agents', 'relations', 'conversations'].includes(section)) return;
    const title = section === 'agents' ? display(selected.name) : section === 'relations' ? `${display(selected.agent_a_name)} ↔ ${display(selected.agent_b_name)}` : display(selected.name || selected.id);
    if (!window.confirm(`确认永久删除「${title}」？此操作不可撤销，并会写入管理审计。`)) return;
    try {
      if (section === 'agents') await request(`/agents/${encodeURIComponent(String(selected.agent_id))}`, { method: 'DELETE' });
      if (section === 'conversations') await request(`/conversations/${encodeURIComponent(String(selected.id))}`, { method: 'DELETE' });
      if (section === 'relations') await request(`/relations/${encodeURIComponent(String(selected.agent_a))}/${encodeURIComponent(String(selected.agent_b))}`, { method: 'DELETE' });
      setSelected(null); setMessages([]); await refresh();
    } catch (cause) { setError(cause instanceof Error ? cause.message : '删除失败'); }
  }

  if (!identity || identity.must_change_password) return <div className="auth"><div className="login panel"><div className="eyebrow">◈ PINGO / CLOUD CONTROL</div><h1>{identity ? '首次登录 · 更换密码' : 'Hub 管理控制台'}</h1><p>只读取 Hub 云端数据，不连接用户 Sidecar。</p><form onSubmit={identity ? changePassword : login}>{!identity && <label>管理员账号<input value={username} onChange={event => setUsername(event.target.value)} required /></label>}<label>{identity ? '当前密码' : '密码'}<input type="password" value={password} onChange={event => setPassword(event.target.value)} required /></label>{identity && <label>新密码<input type="password" value={newPassword} onChange={event => setNewPassword(event.target.value)} /></label>}{error && <div className="error">{error}</div>}<button className="primary">{identity ? '更换密码 →' : '登录 →'}</button></form></div></div>;

  const current = sections.find(item => item.id === section)!;
  const creatorID = selected?.created_by;
  // @ts-expect-error The compressed table row uses a composite fallback key.
  return <div className="shell"><div className="scanlines" /><header className="topbar"><span className="logo">◈</span><div className="brand">PINGO / HUB ADMIN<small>云端管理控制台</small></div><div className="grow" /><span className="live"><i /> LIVE · :18080</span><span className="eyebrow">{identity.username}</span><button className="action" onClick={async () => { await request('/logout', { method: 'POST' }); setIdentity(null); }}>退出</button></header><div className="layout"><nav className="sidebar panel"><div className="eyebrow">01 / NAVIGATION</div>{sections.map(item => <button className={section === item.id ? 'active' : ''} key={item.id} onClick={() => { setSection(item.id); setPage(0); setQuery(''); setSearch(''); setSelected(null); }}><span>{item.glyph}</span>{item.label}</button>)}<div className="nav-foot">HUB DATA ONLY<br />NO SIDECAR · NO MOCK</div></nav><main><div className="heading"><div><div className="eyebrow">CLOUD / {section.toUpperCase()}</div><h1>{current.label}</h1><p>{current.hint}</p></div><button className="action" onClick={() => void refresh()}>↻ 刷新</button></div>{error && <div className="error" role="alert">Hub 操作失败：{error}</div>}{section === 'overview' ? <div className="metrics">{stats && Object.entries(stats).filter(([key]) => key !== 'collected_at').map(([key, value]) => <article className="metric panel" key={key}><span>{key.replaceAll('_', ' ').toUpperCase()}</span><strong>{display(value).padStart(2, '0')}</strong><small>HUB / POSTGRESQL</small></article>)}{loading && !stats && <div className="empty">正在读取 Hub 数据…</div>}</div> : <><form className="toolbar panel" onSubmit={event => { event.preventDefault(); setPage(0); setQuery(search.trim()); }}><input value={search} onChange={event => setSearch(event.target.value)} disabled={!['agents', 'conversations', 'relations'].includes(section)} placeholder={section === 'relations' ? '⌕ Agent ID，回车筛选' : '⌕ 名称或 ID，回车查询'} /><span>每页 30 条 · 第 {page + 1} 页</span></form><div className="table-wrap panel"><table><thead><tr>{columns[section].map(column => <th key={column.key}>{column.label}</th>)}</tr></thead><tbody>{rows.map((row, index) => <tr key={String(row.id ?? row.agent_id ?? row.message_id ?? `${row.agent_a}-${row.agent_b}` ?? index)} onClick={() => void open(row)}>{columns[section].map(column => <td key={column.key}>{column.key === 'online' ? <span className={`state ${row.online ? 'on' : 'off'}`}>{display(row.online)}</span> : column.key === 'name' && section === 'conversations' ? display(row.name || row.member_names || row.id) : display(row[column.key])}</td>)}</tr>)}</tbody></table>{!loading && rows.length === 0 && <div className="empty">∅<p>没有符合条件的云端记录</p></div>}</div><div className="pager"><button disabled={page === 0} onClick={() => setPage(page - 1)}>← 上一页</button><span>{loading ? '正在读取 Hub…' : `已加载 ${rows.length} 条`}</span><button disabled={rows.length < 30} onClick={() => setPage(page + 1)}>下一页 →</button></div></>}</main></div>{selected && <div className="scrim" onClick={() => setSelected(null)}><aside className={`drawer panel ${section === 'conversations' ? 'chat-drawer' : ''}`} onClick={event => event.stopPropagation()}><div className="drawer-actions"><button className="action danger" onClick={() => void removeSelected()}>⌫ 删除</button><button className="action close" onClick={() => setSelected(null)}>×</button></div>{section === 'agents' && <><div className="profile-head"><div className="avatar">{initials(selected.name)}</div><div><div className="eyebrow">AGENT IDENTITY</div><h2>{display(selected.name)}</h2><p>{display(selected.status_text)}</p></div><span className={`state ${selected.online ? 'on' : 'off'}`}>{selected.online ? '在线' : '离线'}</span></div><div className="section-label">基础资料</div><div className="field-grid"><Field label="Agent ID" value={selected.agent_id} /><Field label="主人" value={selected.owner_name} /><Field label="头像地址" value={selected.avatar_url} /><Field label="创建时间" value={selected.created_at} /><Field label="最后心跳" value={selected.last_heartbeat} /></div><div className="section-label">能力标签</div><JsonCards value={selected.capabilities} empty="这个 Agent 还没有公开能力标签" /><div className="section-label">接单偏好</div><JsonCards value={selected.availability} empty="这个 Agent 还没有设置接单偏好" /></>}{section === 'relations' && <><div className="eyebrow">RELATIONSHIP INSPECTOR</div><h2>好友关系</h2><div className="relation-pair"><div className="person-card"><div className="avatar">{initials(selected.agent_a_name)}</div><b>{display(selected.agent_a_name)}</b><code>{display(selected.agent_a)}</code></div><div className="relation-line"><span>◇</span><b>{display(selected.status)}</b></div><div className="person-card"><div className="avatar">{initials(selected.agent_b_name)}</div><b>{display(selected.agent_b_name)}</b><code>{display(selected.agent_b)}</code></div></div><div className="field-grid"><Field label="发起方" value={selected.initiated_by_name || selected.initiated_by} /><Field label="申请说明" value={selected.request_message} /><Field label="互动次数" value={selected.interaction_count} /><Field label="最近互动" value={selected.last_interaction} /><Field label="建立时间" value={selected.created_at} /><Field label="更新时间" value={selected.updated_at} /></div></>}{section === 'conversations' && <><div className="chat-head"><div><div className="eyebrow">CONVERSATION ARCHIVE</div><h2>{display(selected.name || selected.member_names || selected.id)}</h2><p>{display(selected.member_names)}</p></div><div className="chat-stats"><b>{display(selected.member_count)}</b><span>成员</span><b>{display(selected.message_count)}</b><span>消息</span></div></div><div className="chat-meta"><span>{display(selected.type)}</span><span>{display(selected.status)}</span><code>{display(selected.id)}</code></div><div className="chat-stream">{messages.map(message => message.message_type === 'system' ? <div className="system-message" key={String(message.message_id)}>{display(message.content_text)}</div> : <article className={`bubble-row ${message.from_agent === creatorID ? 'mine' : ''}`} key={String(message.message_id)}><div className="mini-avatar">{initials(message.from_name)}</div><div><small>{display(message.from_name)} · {display(message.created_at)}</small><div className="bubble"><p>{display(message.content_text)}</p>{Boolean(message.content_file) && <code>{display(message.content_file)}</code>}</div></div></article>)}{messages.length === 0 && <div className="empty">此会话没有消息</div>}</div></>}{!['agents', 'relations', 'conversations'].includes(section) && <><div className="eyebrow">RECORD INSPECTOR</div><h2>{display(selected.name ?? selected.id ?? selected.agent_id ?? selected.message_id ?? selected.action)}</h2><div className="field-grid">{Object.entries(selected).map(([key, value]) => <Field key={key} label={key} value={value} />)}</div></>}</aside></div>}<footer><span>◈ PINGO / CLOUD</span><span>{current.label}</span><span>只读取 Hub 数据 · 管理操作全部审计</span></footer></div>;
}

export default App;
