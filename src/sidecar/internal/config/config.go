package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Path     string         `yaml:"-"`
	Hub      HubConfig      `yaml:"-"`
	Agents   []AgentConfig  `yaml:"agents"`
	Local    LocalConfig    `yaml:"local"`
	Notify   NotifyConfig   `yaml:"notify"`
	Approval ApprovalConfig `yaml:"approval"`
}

type HubConfig struct {
	Endpoint string `yaml:"endpoint"`
}

var BuiltinHubEndpoint string
var Version = "dev"
var Channel = "dev"
var UpdateManifestURL string
var UpdateCheckInterval = "6h"

func DefaultHubEndpoint() string {
	if BuiltinHubEndpoint != "" {
		return BuiltinHubEndpoint
	}
	return "ws://localhost:18080/ws"
}

type AgentConfig struct {
	ID           string           `yaml:"id"`
	Token        string           `yaml:"token"`
	Name         string           `yaml:"name"`
	OwnerName    string           `yaml:"owner_name"`
	OwnerEmail   string           `yaml:"owner_email"`
	StatusText   string           `yaml:"status_text"`
	Capabilities []CapabilityItem `yaml:"capabilities"`
	Availability AvailabilityConf `yaml:"availability"`
}

type CapabilityItem struct {
	Skill string   `yaml:"skill" json:"skill"`
	Tags  []string `yaml:"tags" json:"tags"`
}

type AvailabilityConf struct {
	Mode                       string `yaml:"mode" json:"mode"`
	OnlineHours                string `yaml:"online_hours" json:"online_hours"`
	Timezone                   string `yaml:"timezone" json:"timezone"`
	MaxConcurrentConversations int    `yaml:"max_concurrent_conversations" json:"max_concurrent_conversations"`
}

type LocalConfig struct {
	APIPort       int    `yaml:"api_port"`
	DashboardPort int    `yaml:"dashboard_port"`
	DBPath        string `yaml:"db_path"`
}

type NotifyConfig struct {
	Terminal bool          `yaml:"terminal"`
	Desktop  bool          `yaml:"desktop"`
	Webhook  WebhookConfig `yaml:"webhook"`
}

type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

type ApprovalConfig struct {
	NewDirectChat  ApprovalRuleConfig  `yaml:"new_direct_chat"`
	NewGroupInvite ApprovalRuleConfig  `yaml:"new_group_invite"`
	HumanDecision  HumanDecisionConfig `yaml:"human_decision"`
}

type ApprovalRuleConfig struct {
	FromTrusted      string `yaml:"from_trusted"`
	FromNormalFriend string `yaml:"from_normal_friend"`
	DefaultTimeout   string `yaml:"default_timeout"`
	TimeoutAction    string `yaml:"timeout_action"`
}

type HumanDecisionConfig struct {
	DefaultTimeout   string           `yaml:"default_timeout"`
	TimeoutAction    string           `yaml:"timeout_action"`
	NotifyEscalation []EscalationRule `yaml:"notify_escalation"`
}

type EscalationRule struct {
	After   string `yaml:"after"`
	Channel string `yaml:"channel"`
}

func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pingo", "config.yaml")
}

func DefaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pingo", "sidecar.db")
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := defaultConfig()
			cfg.Path = path
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// 设置默认值
	applyDefaults(&cfg)
	cfg.Path = path

	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if path == "" {
		path = cfg.Path
	}
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	cfg.Path = path
	return nil
}

func defaultConfig() *Config {
	cfg := &Config{
		Hub: HubConfig{
			Endpoint: DefaultHubEndpoint(),
		},
		Agents: []AgentConfig{},
		Local: LocalConfig{
			APIPort:       19191,
			DashboardPort: 19192,
			DBPath:        DefaultDBPath(),
		},
		Notify: NotifyConfig{
			Terminal: true,
			Desktop:  true,
			Webhook: WebhookConfig{
				Enabled: false,
			},
		},
		Approval: ApprovalConfig{
			NewDirectChat: ApprovalRuleConfig{
				FromTrusted:      "auto_accept",
				FromNormalFriend: "ask_human",
				DefaultTimeout:   "5m",
				TimeoutAction:    "hold",
			},
			NewGroupInvite: ApprovalRuleConfig{
				FromTrusted:      "auto_accept",
				FromNormalFriend: "ask_human",
				DefaultTimeout:   "10m",
				TimeoutAction:    "hold",
			},
			HumanDecision: HumanDecisionConfig{
				DefaultTimeout: "10m",
				TimeoutAction:  "reject",
			},
		},
	}
	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Local.APIPort == 0 {
		cfg.Local.APIPort = 19191
	}
	if cfg.Local.DashboardPort == 0 {
		cfg.Local.DashboardPort = 19192
	}
	if cfg.Local.DBPath == "" {
		cfg.Local.DBPath = DefaultDBPath()
	}
	if cfg.Hub.Endpoint == "" {
		cfg.Hub.Endpoint = DefaultHubEndpoint()
	}
}

// HTTPEndpoint 从 WS endpoint 推导出 HTTP endpoint
func (h HubConfig) HTTPEndpoint() string {
	ws := h.Endpoint
	if ws == "" {
		return "http://localhost:18080"
	}
	// 替换 ws:// -> http://, wss:// -> https://
	if len(ws) >= 5 && ws[:5] == "wss://" {
		rest := ws[5:]
		// 去掉路径部分
		for i, c := range rest {
			if c == '/' {
				rest = rest[:i]
				break
			}
		}
		return "https://" + rest
	}
	if len(ws) >= 5 && ws[:5] == "ws://" {
		rest := ws[5:]
		for i, c := range rest {
			if c == '/' {
				rest = rest[:i]
				break
			}
		}
		return "http://" + rest
	}
	return ws
}
