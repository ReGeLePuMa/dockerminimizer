package agent

import "context"

func AgentFactory(ctx context.Context, maker string, model string, token string) Agent {
	switch maker {
	case "google":
		return NewGoogleAIAgent(ctx, model, token)
	default:
		return nil
	}
}
