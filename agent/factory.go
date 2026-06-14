package agent

import "context"

var agents = map[string]func(context.Context, string, string) Agent{
	"google": NewGoogleAIAgent,
}

func AgentFactory(ctx context.Context, maker string, model string, token string) Agent {
	if agent, exists := agents[maker]; exists {
		return agent(ctx, model, token)
	}
	return nil
}
