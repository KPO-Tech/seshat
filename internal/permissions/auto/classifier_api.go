// Package auto - Classifier API client.
//
// This module provides the API client interface for making classification
// requests to the LLM provider. It wraps the Nexus providers.Client to provide
// a classifier-specific interface with proper request/response handling.
//
// The ClassifierAPI interface defines the contract for classifier API clients,
// allowing for different implementations (e.g., mock for testing, real for production).
package auto

import (
	"context"
	"math/rand"

	"github.com/EngineerProjects/nexus-engine/internal/providers"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ClassifierAPI interface defines the contract for classifier API clients.
// Implementations must provide a Classify method that processes classification requests.
type ClassifierAPI interface {
	Classify(ctx context.Context, req *ClassifierAPIRequest) (*ClassifierAPIResponse, error)
}

type ClassifierAPIRequest struct {
	Model         string
	MaxTokens     int
	System        string
	Temperature   float64
	Messages      []types.Message
	StopSequences []string
	Thinking      bool
}

type ClassifierAPIResponse struct {
	Text       string
	Usage      *ClassifierUsage
	MessageID  string
	RequestID  string
	StopReason string
}

type ClassifierAPIClient struct {
	client *providers.Client
}

func NewClassifierAPIClient(client *providers.Client) *ClassifierAPIClient {
	return &ClassifierAPIClient{
		client: client,
	}
}

func (c *ClassifierAPIClient) Classify(ctx context.Context, req *ClassifierAPIRequest) (*ClassifierAPIResponse, error) {
	messages := append(make([]types.Message, 0, len(req.Messages)), req.Messages...)

	lastMsg := messages[len(messages)-1]
	userContent := lastMsg.Content
	messages = messages[:len(messages)-1]

	userMsg := types.Message{
		ID:      types.MessageID("classifier-user-" + generateID()),
		Role:    types.RoleUser,
		Content: userContent,
	}
	messages = append(messages, userMsg)

	systemPrompt := req.System
	if systemPrompt == "" {
		systemPrompt = "You are a helpful assistant."
	}

	temp := req.Temperature

	apiReq := types.APIRequest{
		Model: types.ModelIdentifier{
			Provider: types.APIProviderAnthropic,
			Model:    req.Model,
		},
		MaxTokens:     req.MaxTokens,
		Temperature:   &temp,
		SystemPrompt:  systemPrompt,
		Messages:      messages,
		StopSequences: req.StopSequences,
	}

	response, err := c.client.CreateMessage(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	var text string
	for _, block := range response.Content {
		if tc, ok := block.(types.TextContent); ok {
			text = tc.Text
			break
		}
	}

	usage := &ClassifierUsage{
		InputTokens:  0,
		OutputTokens: 0,
	}
	if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
		usage.InputTokens = response.Usage.InputTokens
		usage.OutputTokens = response.Usage.OutputTokens
	}

	return &ClassifierAPIResponse{
		Text:       text,
		Usage:      usage,
		MessageID:  string(response.ID),
		StopReason: response.StopReason,
	}, nil
}

func generateID() string {
	return "msg-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
