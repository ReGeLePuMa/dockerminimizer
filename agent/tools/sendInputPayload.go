package tools

import (
	"fmt"
	"io"
)

func WriteToStdinTool(stdin io.Writer) Tool {
	return Tool{
		Name:        "writeToStdin",
		Description: "Write raw string payloads directly into the standard input (stdin) of the main running container process. Use this if the target application is a CLI tool expecting piped data.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"payload": map[string]interface{}{
					"type":        "string",
					"description": "The string or data payload to pipe into the container's standard input.",
				},
			},
			"required": []string{"payload"},
		},
		Execute: func(_ string, input map[string]interface{}) (string, error) {
			if stdin == nil {
				return "", fmt.Errorf("stdin pipe is not connected or available")
			}

			payload, ok := input["payload"].(string)
			if !ok {
				return "", fmt.Errorf("missing or invalid 'payload' parameter")
			}

			bytesWritten, err := stdin.Write([]byte(payload + "\n"))
			if err != nil {
				return "", fmt.Errorf("failed to write to stdin stream: %w", err)
			}
			return fmt.Sprintf("Successfully wrote %d bytes to standard input.", bytesWritten), nil
		},
	}
}
