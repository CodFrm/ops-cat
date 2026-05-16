package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ModelInfo 从 API 返回的模型信息
type ModelInfo struct {
	ID string `json:"id"`
}

// FetchModels 从 API 获取可用模型列表
func FetchModels(providerType, apiBase, apiKey string) ([]ModelInfo, error) {
	switch providerType {
	case "anthropic":
		return fetchAnthropicModels(apiBase, apiKey)
	default:
		return fetchOpenAIModels(apiBase, apiKey)
	}
}

func fetchOpenAIModels(apiBase, apiKey string) ([]ModelInfo, error) {
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	apiBase = strings.TrimRight(apiBase, "/")

	req, err := http.NewRequest("GET", apiBase+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelInfo{ID: m.ID})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

func fetchAnthropicModels(apiBase, apiKey string) ([]ModelInfo, error) {
	if apiBase == "" {
		apiBase = "https://api.anthropic.com"
	}
	apiBase = strings.TrimRight(apiBase, "/")

	req, err := http.NewRequest("GET", apiBase+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelInfo{ID: m.ID})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}
