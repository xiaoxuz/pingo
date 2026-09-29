package approval

// RuleEngine 审批规则引擎
type RuleEngine struct {
	config *Config
}

type Config struct {
	NewDirectChat  RuleConfig
	NewGroupInvite RuleConfig
	HumanDecision  HumanDecisionConfig
}

type RuleConfig struct {
	FromTrusted      string // auto_accept / ask_human / auto_reject
	FromNormalFriend string
	DefaultTimeout   string
	TimeoutAction    string // hold / accept / reject
}

type HumanDecisionConfig struct {
	DefaultTimeout string
	TimeoutAction  string
}

func NewRuleEngine(cfg *Config) *RuleEngine {
	return &RuleEngine{config: cfg}
}

// EvaluateNewChat 评估新单聊请求
func (r *RuleEngine) EvaluateNewChat(trustLevel string) string {
	switch trustLevel {
	case "trusted":
		return r.config.NewDirectChat.FromTrusted
	default:
		return r.config.NewDirectChat.FromNormalFriend
	}
}

// EvaluateGroupInvite 评估群邀请
func (r *RuleEngine) EvaluateGroupInvite(trustLevel string) string {
	switch trustLevel {
	case "trusted":
		return r.config.NewGroupInvite.FromTrusted
	default:
		return r.config.NewGroupInvite.FromNormalFriend
	}
}

// GetTimeout 获取超时时间
func (r *RuleEngine) GetTimeout(approvalType string) string {
	switch approvalType {
	case "new_chat":
		return r.config.NewDirectChat.DefaultTimeout
	case "group_invite":
		return r.config.NewGroupInvite.DefaultTimeout
	case "human_decision":
		return r.config.HumanDecision.DefaultTimeout
	default:
		return "5m"
	}
}

// GetTimeoutAction 获取超时动作
func (r *RuleEngine) GetTimeoutAction(approvalType string) string {
	switch approvalType {
	case "new_chat":
		return r.config.NewDirectChat.TimeoutAction
	case "group_invite":
		return r.config.NewGroupInvite.TimeoutAction
	case "human_decision":
		return r.config.HumanDecision.TimeoutAction
	default:
		return "hold"
	}
}
