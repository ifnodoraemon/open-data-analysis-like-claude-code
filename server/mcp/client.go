package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ToolSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type ServerConfig struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	AuthToken string `json:"auth_token,omitempty"`
}

type Client struct {
	configs map[string]ServerConfig
	mu      sync.RWMutex
}

func NewClient() *Client {
	return &Client{
		configs: make(map[string]ServerConfig),
	}
}

func (c *Client) RegisterServer(name, endpoint, authToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs[name] = ServerConfig{
		Name:      name,
		Endpoint:  strings.TrimRight(endpoint, "/"),
		AuthToken: authToken,
	}
}

func (c *Client) RemoveServer(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.configs, name)
}

func (c *Client) ListServers() []ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]ServerConfig, 0, len(c.configs))
	for _, cfg := range c.configs {
		result = append(result, cfg)
	}
	return result
}

func (c *Client) DiscoverTools(ctx context.Context) ([]ToolSchema, error) {
	servers := c.ListServers()
	var allTools []ToolSchema
	var errs []string

	for _, srv := range servers {
		tools, err := c.discoverFromServer(ctx, srv)
		if err != nil {
			log.Printf("mcp: discover tools from %s failed: %v", srv.Name, err)
			errs = append(errs, fmt.Sprintf("%s: %v", srv.Name, err))
			continue
		}
		allTools = append(allTools, tools...)
	}

	if len(errs) > 0 && len(allTools) == 0 {
		return nil, fmt.Errorf("all MCP servers failed: %s", strings.Join(errs, "; "))
	}

	return allTools, nil
}

func (c *Client) discoverFromServer(ctx context.Context, srv ServerConfig) ([]ToolSchema, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.Endpoint+"/tools", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if srv.AuthToken != "" {
		req.Header.Set("X-Proxy-Token", srv.AuthToken)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Tools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			Parameters  map[string]interface{} `json:"parameters"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	tools := make([]ToolSchema, 0, len(result.Tools))
	for _, t := range result.Tools {
		tools = append(tools, ToolSchema{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	return tools, nil
}

func (c *Client) ExecuteTool(ctx context.Context, serverName, toolName string, args json.RawMessage) (json.RawMessage, error) {
	c.mu.RLock()
	srv, ok := c.configs[serverName]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", serverName)
	}

	payload := map[string]interface{}{
		"tool_name": toolName,
		"args":      args,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.Endpoint+"/execute-tool", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if srv.AuthToken != "" {
		req.Header.Set("X-Proxy-Token", srv.AuthToken)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return json.RawMessage(respBody), nil
}
