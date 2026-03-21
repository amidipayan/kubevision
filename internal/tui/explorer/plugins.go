package explorer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)


type PluginConfig struct {
	Plugins map[string][]Plugin `yaml:"plugins"` 
}


type Plugin struct {
	ShortCut    string `yaml:"shortCut"`    
	Description string `yaml:"description"` 
	Command     string `yaml:"command"`     
	Background  bool   `yaml:"background"`  
}


func LoadPlugins() (map[string][]Plugin, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(home, ".kubevision", "plugins.yaml")
	
	
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return make(map[string][]Plugin), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugins.yaml: %w", err)
	}

	var config PluginConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse plugins.yaml: %w", err)
	}

	
	normalized := make(map[string][]Plugin)
	for k, v := range config.Plugins {
		normalized[strings.ToLower(k)] = v
	}

	return normalized, nil
}


func (p Plugin) InterpolateCommand(r Resource) string {
	cmd := p.Command
	cmd = strings.ReplaceAll(cmd, "{{name}}", r.Name)
	cmd = strings.ReplaceAll(cmd, "{{namespace}}", r.Namespace)
	cmd = strings.ReplaceAll(cmd, "{{kind}}", strings.ToLower(r.Kind))
	return cmd
}