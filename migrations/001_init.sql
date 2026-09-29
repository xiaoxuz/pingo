-- Pingo 初始化数据库

-- ============================================================
-- Agent 表
-- ============================================================
CREATE TABLE agents (
    agent_id        VARCHAR(64) PRIMARY KEY,
    name            VARCHAR(128) NOT NULL,
    token           VARCHAR(256) NOT NULL,

    owner_name      VARCHAR(128),
    owner_email     VARCHAR(256),

    avatar_url      VARCHAR(512),
    status_text     VARCHAR(512),

    capabilities    JSONB NOT NULL DEFAULT '[]',
    availability    JSONB NOT NULL DEFAULT '{}',

    online          BOOLEAN DEFAULT FALSE,
    last_heartbeat  TIMESTAMP,

    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_agents_online ON agents(online);

-- ============================================================
-- 能力索引表（用于搜索发现）
-- ============================================================
CREATE TABLE capability_index (
    agent_id    VARCHAR(64) REFERENCES agents(agent_id) ON DELETE CASCADE,
    skill       VARCHAR(128),
    tag         VARCHAR(128),
    PRIMARY KEY (agent_id, skill, tag)
);

CREATE INDEX idx_cap_skill ON capability_index(skill);
CREATE INDEX idx_cap_tag ON capability_index(tag);

-- ============================================================
-- 好友关系表
-- ============================================================
CREATE TABLE friendships (
    id              SERIAL PRIMARY KEY,
    agent_a         VARCHAR(64) REFERENCES agents(agent_id),
    agent_b         VARCHAR(64) REFERENCES agents(agent_id),
    status          VARCHAR(20) NOT NULL,
    initiated_by    VARCHAR(64),
    request_message VARCHAR(512),

    a_nickname_for_b    VARCHAR(128),
    a_group_for_b       VARCHAR(64),
    a_trust_level       VARCHAR(20) DEFAULT 'normal',
    a_trust_rules       JSONB DEFAULT '{}',

    b_nickname_for_a    VARCHAR(128),
    b_group_for_a       VARCHAR(64),
    b_trust_level       VARCHAR(20) DEFAULT 'normal',
    b_trust_rules       JSONB DEFAULT '{}',

    interaction_count   INTEGER DEFAULT 0,
    last_interaction    TIMESTAMP,

    created_at          TIMESTAMP DEFAULT NOW(),
    updated_at          TIMESTAMP DEFAULT NOW(),

    UNIQUE(agent_a, agent_b),
    CHECK(agent_a < agent_b)
);

CREATE INDEX idx_friendship_a ON friendships(agent_a);
CREATE INDEX idx_friendship_b ON friendships(agent_b);
CREATE INDEX idx_friendship_status ON friendships(status);

-- ============================================================
-- 好友分组定义表
-- ============================================================
CREATE TABLE friend_groups (
    agent_id    VARCHAR(64) REFERENCES agents(agent_id) ON DELETE CASCADE,
    group_name  VARCHAR(64),
    sort_order  INTEGER DEFAULT 0,
    PRIMARY KEY (agent_id, group_name)
);

-- ============================================================
-- 会话表
-- ============================================================
CREATE TABLE conversations (
    conversation_id VARCHAR(64) PRIMARY KEY,
    type            VARCHAR(10) NOT NULL,
    name            VARCHAR(256),
    created_by      VARCHAR(64) REFERENCES agents(agent_id),
    status          VARCHAR(20) DEFAULT 'active',

    created_at      TIMESTAMP DEFAULT NOW(),
    closed_at       TIMESTAMP,

    last_message_at     TIMESTAMP,
    last_message_preview VARCHAR(200)
);

CREATE INDEX idx_conv_status ON conversations(status);
CREATE INDEX idx_conv_created_by ON conversations(created_by);

-- ============================================================
-- 会话成员表
-- ============================================================
CREATE TABLE conversation_members (
    conversation_id VARCHAR(64) REFERENCES conversations(conversation_id) ON DELETE CASCADE,
    agent_id        VARCHAR(64) REFERENCES agents(agent_id),
    role            VARCHAR(20) DEFAULT 'member',
    joined_at       TIMESTAMP DEFAULT NOW(),
    left_at         TIMESTAMP,

    last_read_message_id VARCHAR(64),
    unread_count         INTEGER DEFAULT 0,

    PRIMARY KEY (conversation_id, agent_id)
);

CREATE INDEX idx_conv_member_agent ON conversation_members(agent_id);
CREATE INDEX idx_conv_member_unread ON conversation_members(agent_id, unread_count) WHERE unread_count > 0;

-- ============================================================
-- 消息表
-- ============================================================
CREATE TABLE messages (
    message_id      VARCHAR(64) PRIMARY KEY,
    conversation_id VARCHAR(64) REFERENCES conversations(conversation_id) ON DELETE CASCADE,
    from_agent      VARCHAR(64) REFERENCES agents(agent_id),

    message_type    VARCHAR(20) NOT NULL,

    content_text    TEXT,
    content_file    JSONB,

    mentions        VARCHAR(64)[],
    reply_to        VARCHAR(64),

    created_at      TIMESTAMP DEFAULT NOW(),

    system_event    VARCHAR(30)
);

CREATE INDEX idx_msg_conv ON messages(conversation_id, created_at);
CREATE INDEX idx_msg_from ON messages(from_agent);

-- ============================================================
-- 离线消息队列
-- ============================================================
CREATE TABLE offline_queue (
    id              SERIAL PRIMARY KEY,
    target_agent    VARCHAR(64) REFERENCES agents(agent_id) ON DELETE CASCADE,
    message_id      VARCHAR(64) REFERENCES messages(message_id) ON DELETE CASCADE,
    conversation_id VARCHAR(64),
    queued_at       TIMESTAMP DEFAULT NOW(),
    delivered       BOOLEAN DEFAULT FALSE,
    delivered_at    TIMESTAMP
);

CREATE INDEX idx_offline_target ON offline_queue(target_agent, delivered) WHERE NOT delivered;
CREATE INDEX idx_offline_queued ON offline_queue(queued_at);
