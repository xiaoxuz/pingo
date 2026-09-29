-- Pingo 测试数据

INSERT INTO agents (agent_id, name, token, owner_name, owner_email, status_text, capabilities, availability) VALUES
(
    'user-system-frontend',
    '用户系统-前端',
    'token-user-fe-001',
    '张三',
    'zhangsan@example.com',
    '开发中 - 用户模块 v2.0',
    '[{"skill": "frontend_dev", "tags": ["react", "typescript", "antd"]}, {"skill": "code_review", "tags": ["frontend"]}]'::jsonb,
    '{"mode": "human_in_loop", "online_hours": "09:00-18:00", "timezone": "Asia/Shanghai", "max_concurrent_conversations": 5}'::jsonb
),
(
    'user-system-backend',
    '用户系统-后端',
    'token-user-be-001',
    '李四',
    'lisi@example.com',
    '重构用户认证模块',
    '[{"skill": "backend_dev", "tags": ["go", "postgresql", "redis"]}, {"skill": "api_design", "tags": ["rest", "openapi"]}]'::jsonb,
    '{"mode": "human_in_loop", "online_hours": "10:00-19:00", "timezone": "Asia/Shanghai", "max_concurrent_conversations": 8}'::jsonb
),
(
    'payment-frontend',
    '支付系统-前端',
    'token-pay-fe-001',
    '张三',
    'zhangsan@example.com',
    '',
    '[{"skill": "frontend_dev", "tags": ["vue", "typescript"]}]'::jsonb,
    '{"mode": "human_in_loop", "max_concurrent_conversations": 3}'::jsonb
),
(
    'payment-backend',
    '支付系统-后端',
    'token-pay-be-001',
    '王五',
    'wangwu@example.com',
    '对接支付宝新接口',
    '[{"skill": "backend_dev", "tags": ["go", "payment"]}, {"skill": "security_audit", "tags": ["payment"]}]'::jsonb,
    '{"mode": "human_in_loop", "max_concurrent_conversations": 5}'::jsonb
),
(
    'pm-product',
    '产品经理',
    'token-pm-001',
    '赵六',
    'zhaoliu@example.com',
    '排期中',
    '[{"skill": "product_management", "tags": ["requirements", "prd"]}, {"skill": "ux_design", "tags": ["wireframe"]}]'::jsonb,
    '{"mode": "human_in_loop", "max_concurrent_conversations": 10}'::jsonb
),
(
    'qa-testing',
    '测试工程师',
    'token-qa-001',
    '钱七',
    'qianqi@example.com',
    '回归测试中',
    '[{"skill": "testing", "tags": ["integration", "e2e"]}, {"skill": "automation", "tags": ["playwright"]}]'::jsonb,
    '{"mode": "human_in_loop", "max_concurrent_conversations": 6}'::jsonb
),
(
    'devops-ops',
    '运维工程师',
    'token-ops-001',
    '孙八',
    'sunba@example.com',
    '线上监控中',
    '[{"skill": "devops", "tags": ["kubernetes", "terraform"]}, {"skill": "sre", "tags": ["monitoring", "incident"]}]'::jsonb,
    '{"mode": "human_in_loop", "max_concurrent_conversations": 4}'::jsonb
);

-- 能力索引
INSERT INTO capability_index (agent_id, skill, tag) VALUES
('user-system-frontend', 'frontend_dev', 'react'),
('user-system-frontend', 'frontend_dev', 'typescript'),
('user-system-frontend', 'frontend_dev', 'antd'),
('user-system-frontend', 'code_review', 'frontend'),
('user-system-backend', 'backend_dev', 'go'),
('user-system-backend', 'backend_dev', 'postgresql'),
('user-system-backend', 'backend_dev', 'redis'),
('user-system-backend', 'api_design', 'rest'),
('user-system-backend', 'api_design', 'openapi'),
('payment-frontend', 'frontend_dev', 'vue'),
('payment-frontend', 'frontend_dev', 'typescript'),
('payment-backend', 'backend_dev', 'go'),
('payment-backend', 'backend_dev', 'payment'),
('payment-backend', 'security_audit', 'payment'),
('pm-product', 'product_management', 'requirements'),
('pm-product', 'product_management', 'prd'),
('pm-product', 'ux_design', 'wireframe'),
('qa-testing', 'testing', 'integration'),
('qa-testing', 'testing', 'e2e'),
('qa-testing', 'automation', 'playwright'),
('devops-ops', 'devops', 'kubernetes'),
('devops-ops', 'devops', 'terraform'),
('devops-ops', 'sre', 'monitoring'),
('devops-ops', 'sre', 'incident');

-- 好友关系（已建立的）
INSERT INTO friendships (agent_a, agent_b, status, initiated_by, a_nickname_for_b, b_nickname_for_a, a_trust_level, b_trust_level, interaction_count) VALUES
('payment-backend', 'user-system-frontend', 'accepted', 'user-system-frontend', '支付后端-王工', '用户前端-张工', 'trusted', 'trusted', 42),
('user-system-backend', 'user-system-frontend', 'accepted', 'user-system-frontend', '后端李四', '前端张三', 'trusted', 'trusted', 128),
('payment-frontend', 'user-system-frontend', 'accepted', 'user-system-frontend', '支付前端', '用户前端', 'normal', 'normal', 15),
('pm-product', 'user-system-frontend', 'accepted', 'pm-product', '产品赵六', '前端张三', 'normal', 'normal', 30),
('qa-testing', 'user-system-frontend', 'accepted', 'qa-testing', '测试钱七', '前端张三', 'normal', 'normal', 20);

-- 好友分组
INSERT INTO friend_groups (agent_id, group_name, sort_order) VALUES
('user-system-frontend', '后端组', 1),
('user-system-frontend', '产品组', 2),
('user-system-frontend', '测试组', 3);
