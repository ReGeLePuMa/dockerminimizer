package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/regelepuma/dockerminimizer/agent/tools"
	"google.golang.org/genai"
)

type GoogleAIAgent struct {
	client       *genai.Client
	model        string
	systemPrompt string
	tools        []tools.Tool
}

func NewGoogleAIAgent(ctx context.Context, model string, token string) Agent {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  token,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal("Failed to create Google AI client:", err)
	}
	return &GoogleAIAgent{client: client, model: model}
}

func (a *GoogleAIAgent) RegisterTools(tools []tools.Tool) {
	a.tools = tools
	a.systemPrompt = getSystemPrompt(tools)
}

func (a *GoogleAIAgent) buildSDKTools() []*genai.Tool {
	declarations := make([]*genai.FunctionDeclaration, len(a.tools))
	for i, t := range a.tools {
		properties := map[string]*genai.Schema{}
		required := []string{}

		for name, raw := range t.InputSchema {
			prop, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			schema := &genai.Schema{}
			if desc, ok := prop["description"].(string); ok {
				schema.Description = desc
			}
			switch prop["type"] {
			case "integer":
				schema.Type = genai.TypeInteger
			case "object":
				schema.Type = genai.TypeObject
			default:
				schema.Type = genai.TypeString
			}
			properties[name] = schema
			if req, ok := prop["required"].(bool); ok && req {
				required = append(required, name)
			}
		}

		declarations[i] = &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: properties,
				Required:   required,
			},
		}
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}
}

func (a *GoogleAIAgent) GenerateAction(ctx context.Context, prompt string) (string, error) {
	config := &genai.GenerateContentConfig{}
	const maxAgentReasoningSteps = 20

	if len(a.tools) > 0 {
		config.Tools = a.buildSDKTools()
	}

	history := []*genai.Content{
		{
			Role:  "user",
			Parts: []*genai.Part{{Text: prompt}},
		},
	}

	for i := 0; i < maxAgentReasoningSteps; i++ {
		resp, err := a.client.Models.GenerateContent(ctx, a.model, history, config)
		if err != nil {
			return "", err
		}

		candidate := resp.Candidates[0]
		history = append(history, candidate.Content)

		if candidate.FinishReason == genai.FinishReasonStop {
			return resp.Text(), nil
		}

		var toolResults []*genai.Part
		for _, part := range candidate.Content.Parts {
			fc := part.FunctionCall
			if fc == nil {
				continue
			}

			result, err := a.executeTool(fc.Name, fc.Args)
			if err != nil {
				result = "error: " + err.Error()
			}

			toolResults = append(toolResults, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: map[string]interface{}{"result": result},
				},
			})
		}

		if len(toolResults) == 0 {
			break
		}

		history = append(history, &genai.Content{
			Role:  "user",
			Parts: toolResults,
		})
	}

	return "", nil
}

func (a *GoogleAIAgent) executeTool(name string, input map[string]interface{}) (string, error) {
	for _, t := range a.tools {
		if t.Name == name {

			return t.Execute("", input)
		}
	}
	return "", fmt.Errorf("unknown tool: %s", name)
}
