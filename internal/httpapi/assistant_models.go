package httpapi

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
)

const assistantDefaultModel = "glm-5.3-flash"

type assistantModel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Family  string `json:"family"`
	Credits string `json:"credits"`
}

type assistantModelCatalog struct {
	Default string           `json:"default"`
	Models  []assistantModel `json:"models"`
}

func prefetchAssistantModels() {}

// models returns the assistant model catalog. The list itself carries no
// user-specific data, so it is served to guests too; actually chatting still
// requires login plus a configured CodeBuddy key.
func (h *assistantHandlers) models(c *gin.Context) {
	c.JSON(200, loadAssistantModels(c.Request.Context(), ""))
}

func loadAssistantModels(context.Context, string) assistantModelCatalog {
	return assistantFixedCatalog()
}

func assistantFixedCatalog() assistantModelCatalog {
	models := []assistantModel{
		assistantListedModel("claude-sonnet-5.5", "Claude-Sonnet-5.5", "x1.33"),
		assistantListedModel("claude-sonnet-5", "Claude-Sonnet-5", "x1.33"),
		assistantListedModel("claude-sonnet-5-1m", "Claude-Sonnet-5-1M", "x1.33"),
		assistantListedModel("claude-sonnet-4.6", "Claude-Sonnet-4.6", "x2.00"),
		assistantListedModel("claude-sonnet-4.6-1m", "Claude-Sonnet-4.6-1M", "x2.00"),
		assistantListedModel("claude-opus-5.5", "Claude-Opus-5.5", "x2.17"),
		assistantListedModel("claude-opus-5", "Claude-Opus-5", "x3.33"),
		assistantListedModel("claude-opus-4.8", "Claude-Opus-4.8", "x3.33"),
		assistantListedModel("claude-opus-4.8-1m", "Claude-Opus-4.8-1M", "x3.33"),
		assistantListedModel("claude-opus-4.7", "Claude-Opus-4.7", "x3.33"),
		assistantListedModel("claude-opus-4.7-1m", "Claude-Opus-4.7-1M", "x3.33"),
		assistantListedModel("claude-opus-4.6", "Claude-Opus-4.6", "x3.33"),
		assistantListedModel("claude-opus-4.6-1m", "Claude-Opus-4.6-1M", "x3.33"),
		assistantListedModel("gemini-3.1-pro", "Gemini-3.1-Pro", "x1.32"),
		assistantListedModel("gemini-3.5-flash", "Gemini-3.5-Flash", "x0.99"),
		assistantListedModel("gpt-6.1-sol", "GPT-6.1-Sol", "x1.33"),
		assistantListedModel("gpt-6-astra", "GPT-6-Astra", "x6.67"),
		assistantListedModel("gpt-6-sol", "GPT-6-Sol", "x1.33"),
		assistantListedModel("gpt-6-luna", "GPT-6-Luna", "x0.07"),
		assistantListedModel("gpt-5.6-sol", "GPT-5.6-Sol", "x3.47"),
		assistantListedModel("gpt-5.6-terra", "GPT-5.6-Terra", "x1.39"),
		assistantListedModel("gpt-5.6-luna", "GPT-5.6-Luna", "x0.14"),
		assistantListedModel("gpt-5.5", "GPT-5.5", "x3.31"),
		assistantListedModel("gpt-5.4", "GPT-5.4", "x1.65"),
		assistantListedModel("gpt-5.3-codex", "GPT-5.3-Codex", "x1.25"),
		assistantListedModel("glm-5.3", "GLM-5.3", "x0.79"),
		assistantListedModel("glm-5.3-flash", "GLM-5.3-Flash", "x0.06"),
		assistantListedModel("glm-5.3-flashx", "GLM-5.3-FlashX", "x0.14"),
		assistantListedModel("glm-5.2", "GLM-5.2", "x0.79"),
		assistantListedModel("glm-5v-turbo", "GLM-5v-Turbo", "x0.71"),
		assistantListedModel("minimax-m3", "MiniMax-M3", "x0.25"),
		assistantListedModel("minimax-m2.7", "MiniMax-M2.7", "x0.19"),
		assistantListedModel("kimi-k3", "Kimi-K3", "x1.62"),
		assistantListedModel("kimi-k2.8-preview", "Kimi-K2.8-Preview", "x0.77"),
		assistantListedModel("kimi-k2.7", "Kimi-K2.7-Code", "x0.65"),
		assistantListedModel("kimi-k2.6", "Kimi-K2.6", "x0.52"),
		assistantListedModel("hy4-preview", "Hy4 preview", "x0.00"),
		assistantListedModel("hy3", "Hy3", "x0.00"),
		assistantListedModel("deepseek-v4.1-flash", "Deepseek-V4.1-Flash", "x0.11"),
		assistantListedModel("deepseek-v4-flash", "Deepseek-V4-Flash", "x0.11"),
		assistantListedModel("deepseek-v4-pro", "Deepseek-V4-Pro", "x0.51"),
	}
	return assistantModelCatalog{Default: assistantDefaultModel, Models: models}
}

func assistantListedModel(id, name, credits string) assistantModel {
	return assistantModel{ID: id, Name: name, Family: assistantModelFamily(id), Credits: credits}
}

func assistantModelFamily(id string) string {
	id = strings.ToLower(id)
	switch {
	case strings.HasPrefix(id, "claude"):
		return "claude"
	case strings.HasPrefix(id, "gpt") || strings.Contains(id, "codex"):
		return "gpt"
	case strings.HasPrefix(id, "glm"):
		return "glm"
	case strings.HasPrefix(id, "gemini"):
		return "gemini"
	case strings.HasPrefix(id, "kimi"):
		return "kimi"
	case strings.HasPrefix(id, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(id, "minimax"):
		return "minimax"
	case strings.HasPrefix(id, "hy") || strings.Contains(id, "hunyuan"):
		return "hunyuan"
	default:
		return "other"
	}
}

func assistantAllowedModel(id string, catalog assistantModelCatalog) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return catalog.Default
	}
	for _, model := range catalog.Models {
		if model.ID == id {
			return id
		}
	}
	return catalog.Default
}
