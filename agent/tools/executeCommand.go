package tools

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func ExecuteCommandTool() Tool {
	return Tool{
		Name:        "executeCommand",
		Description: "Execute a shell command inside the target container and capture stdout/stderr",
		InputSchema: map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The full shell command string to execute (e.g., 'ls -la' or 'curl localhost:80')",
				"required":    true,
			},
		},
		Execute: func(containerName string, input map[string]interface{}) (string, error) {
			rawCmd, ok := input["command"].(string)
			if !ok || rawCmd == "" {
				return "", fmt.Errorf("missing or invalid 'command' parameter")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "sh", "-c", rawCmd)

			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Execution failed. Output: %s | Error: %v", string(output), err), nil
			}

			return string(output), nil
		},
	}
}
