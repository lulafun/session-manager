package main

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

type CodexConfig struct {
	ModelProvider  string                 `toml:"model_provider"`
	ModelProviders map[string]interface{} `toml:"model_providers"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".codex", "config.toml")
	}
	return filepath.Join(home, ".codex", "config.toml")
}

func ReadConfigProviders(path string) ([]string, string, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	var cfg CodexConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	providers := make([]string, 0, len(cfg.ModelProviders)+1)
	seen := map[string]bool{}
	if cfg.ModelProvider != "" {
		providers = append(providers, cfg.ModelProvider)
		seen[cfg.ModelProvider] = true
	}
	for provider := range cfg.ModelProviders {
		if provider == "" || seen[provider] {
			continue
		}
		providers = append(providers, provider)
		seen[provider] = true
	}
	if len(providers) > 1 {
		head := providers[0]
		tail := append([]string(nil), providers[1:]...)
		sort.Strings(tail)
		providers = append([]string{head}, tail...)
	}
	return providers, cfg.ModelProvider, nil
}

func MergeProviders(primary []string, secondary []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(primary)+len(secondary))
	for _, provider := range append(primary, secondary...) {
		if provider == "" || provider == "(missing)" || seen[provider] {
			continue
		}
		out = append(out, provider)
		seen[provider] = true
	}
	return out
}
