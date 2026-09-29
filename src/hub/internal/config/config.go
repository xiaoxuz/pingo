package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Database     DatabaseConfig     `yaml:"database"`
	Redis        RedisConfig        `yaml:"redis"`
	Auth         AuthConfig         `yaml:"auth"`
	FileStorage  FileStorageConfig  `yaml:"file_storage"`
	OfflineQueue OfflineQueueConfig `yaml:"offline_queue"`
	Heartbeat    HeartbeatConfig    `yaml:"heartbeat"`
	Connection   ConnectionConfig   `yaml:"connection"`
}

type ServerConfig struct {
	HTTPPort int    `yaml:"http_port"`
	WSPath   string `yaml:"ws_path"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"ssl_mode"`
}

func (c DatabaseConfig) DSN() string {
	sslmode := c.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		c.Host, c.Port, c.Name, c.User, c.Password, sslmode)
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type AuthConfig struct {
	Type string `yaml:"type"`
}

type FileStorageConfig struct {
	Type        string `yaml:"type"`
	LocalPath   string `yaml:"local_path"`
	MaxFileSize string `yaml:"max_file_size"`
}

type OfflineQueueConfig struct {
	MaxAge      string `yaml:"max_age"`
	MaxPerAgent int    `yaml:"max_per_agent"`
}

type HeartbeatConfig struct {
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
}

func (c Heartbeat) IntervalDuration() time.Duration {
	d, _ := time.ParseDuration(c.Interval)
	if d == 0 {
		return 30 * time.Second
	}
	return d
}

func (c Heartbeat) TimeoutDuration() time.Duration {
	d, _ := time.ParseDuration(c.Timeout)
	if d == 0 {
		return 90 * time.Second
	}
	return d
}

// Heartbeat is an alias to match the config field name
type Heartbeat = HeartbeatConfig

type ConnectionConfig struct {
	DuplicatePolicy string `yaml:"duplicate_policy"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// 设置默认值
	if cfg.Server.HTTPPort == 0 {
		cfg.Server.HTTPPort = 18080
	}
	if cfg.Server.WSPath == "" {
		cfg.Server.WSPath = "/ws"
	}
	if cfg.Database.Host == "" {
		cfg.Database.Host = "localhost"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
	if cfg.FileStorage.Type == "" {
		cfg.FileStorage.Type = "local"
	}
	if cfg.FileStorage.LocalPath == "" {
		cfg.FileStorage.LocalPath = "./data/files"
	}
	if cfg.FileStorage.MaxFileSize == "" {
		cfg.FileStorage.MaxFileSize = "50MB"
	}
	if cfg.OfflineQueue.MaxAge == "" {
		cfg.OfflineQueue.MaxAge = "7d"
	}
	if cfg.OfflineQueue.MaxPerAgent == 0 {
		cfg.OfflineQueue.MaxPerAgent = 1000
	}
	if cfg.Heartbeat.Interval == "" {
		cfg.Heartbeat.Interval = "30s"
	}
	if cfg.Heartbeat.Timeout == "" {
		cfg.Heartbeat.Timeout = "90s"
	}
	if cfg.Connection.DuplicatePolicy == "" {
		cfg.Connection.DuplicatePolicy = "kick_old"
	}

	// 环境变量覆盖
	if os.Getenv("DB_HOST") != "" {
		cfg.Database.Host = os.Getenv("DB_HOST")
	}
	if os.Getenv("DB_PORT") != "" {
		fmt.Sscanf(os.Getenv("DB_PORT"), "%d", &cfg.Database.Port)
	}
	if os.Getenv("DB_NAME") != "" {
		cfg.Database.Name = os.Getenv("DB_NAME")
	}
	if os.Getenv("DB_USER") != "" {
		cfg.Database.User = os.Getenv("DB_USER")
	}
	if os.Getenv("DB_PASSWORD") != "" {
		cfg.Database.Password = os.Getenv("DB_PASSWORD")
	}
	if os.Getenv("REDIS_ADDR") != "" {
		cfg.Redis.Addr = os.Getenv("REDIS_ADDR")
	}

	return &cfg, nil
}
