package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port         string `yaml:"port"`
		Env          string `yaml:"env"`
		APIBaseRoute string `yaml:"api_base_route"`
	} `yaml:"server"`

	Backend struct {
		APIURL string `yaml:"api_url"`
	} `yaml:"backend"`

	Database struct {
		Path  string `yaml:"path"`
		Redis struct {
			Addr     string `yaml:"addr"`
			UserName string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"redis"`
	} `yaml:"database"`

	Security struct {
		APIKeySecret string `yaml:"api_key_secret"`
	} `yaml:"security"`

	CORS struct {
		AllowedOrigins string `yaml:"allowed_origins"`
	} `yaml:"cors"`

	LXC struct {
		Bridge string `yaml:"bridge"`
	} `yaml:"lxc"`
}

var AppConfig *Config

func LoadConfig() *Config {
	var configPath string
	if customPath := os.Getenv("CONFIG_PATH"); customPath != "" {
		configPath = customPath
	} else {
		env := getEnv("ENV", "development")
		if env == "production" {
			configPath = "/var/lynx/config.yml"
		} else {
			configPath = "./config.yml"
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config file %s: %v", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}
	if cfg.Server.Env == "" {
		cfg.Server.Env = "development"
	}
	if cfg.Server.APIBaseRoute == "" {
		cfg.Server.APIBaseRoute = "/api/v1"
	}
	if cfg.Backend.APIURL == "" {
		cfg.Backend.APIURL = "http://localhost:5000/api/v1"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./data/agent.db"
	}
	if cfg.Database.Redis.Addr == "" {
		cfg.Database.Redis.Addr = "localhost:6379"
	}
	if cfg.CORS.AllowedOrigins == "" {
		cfg.CORS.AllowedOrigins = "http://localhost:5174"
	}
	if cfg.LXC.Bridge == "" {
		cfg.LXC.Bridge = "lxcbr0"
	}

	AppConfig = &cfg

	log.Printf("✓ Configuration loaded from %s\n", configPath)
	log.Printf("  - Server: %s:%s (env: %s)\n", "0.0.0.0", cfg.Server.Port, cfg.Server.Env)
	log.Printf("  - Database: %s\n", cfg.Database.Path)
	log.Printf("  - Backend API: %s\n", cfg.Backend.APIURL)

	return AppConfig
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (c *Config) GetPort() string          { return c.Server.Port }
func (c *Config) GetEnv() string           { return c.Server.Env }
func (c *Config) GetAPIBaseRoute() string  { return c.Server.APIBaseRoute }
func (c *Config) GetBackendAPIURL() string { return c.Backend.APIURL }
func (c *Config) GetDBPath() string        { return c.Database.Path }
func (c *Config) GetAPIKeySecret() string  { return c.Security.APIKeySecret }
func (c *Config) GetCORSOrigins() string   { return c.CORS.AllowedOrigins }
func (c *Config) GetLXCBridge() string     { return c.LXC.Bridge }
