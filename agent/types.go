package agent

import (
	"context"

	"github.com/regelepuma/dockerminimizer/agent/tools"
)

type Agent interface {
	GenerateAction(ctx context.Context, prompt string) (string, error)
	RegisterTools(tools []tools.Tool)
}

func BoundTools(rawTools []tools.Tool, containerName string) []tools.Tool {
	bound := make([]tools.Tool, len(rawTools))

	for i, t := range rawTools {
		bound[i] = t
		original := t.Execute
		cn := containerName

		bound[i].Execute = func(_ string, input map[string]interface{}) (string, error) {
			return original(cn, input)
		}
	}
	return bound
}
