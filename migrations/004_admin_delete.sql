ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_created_by_fkey;
ALTER TABLE conversations
    ADD CONSTRAINT conversations_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES agents(agent_id) ON DELETE SET NULL;

ALTER TABLE conversation_members DROP CONSTRAINT IF EXISTS conversation_members_agent_id_fkey;
ALTER TABLE conversation_members
    ADD CONSTRAINT conversation_members_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE;

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_from_agent_fkey;
ALTER TABLE messages
    ADD CONSTRAINT messages_from_agent_fkey
    FOREIGN KEY (from_agent) REFERENCES agents(agent_id) ON DELETE SET NULL;

ALTER TABLE friendships DROP CONSTRAINT IF EXISTS friendships_agent_a_fkey;
ALTER TABLE friendships DROP CONSTRAINT IF EXISTS friendships_agent_b_fkey;
ALTER TABLE friendships
    ADD CONSTRAINT friendships_agent_a_fkey
    FOREIGN KEY (agent_a) REFERENCES agents(agent_id) ON DELETE CASCADE;
ALTER TABLE friendships
    ADD CONSTRAINT friendships_agent_b_fkey
    FOREIGN KEY (agent_b) REFERENCES agents(agent_id) ON DELETE CASCADE;
