package tools

import "io"

func GetAvailableTools(stdin io.Writer) []Tool {
	return []Tool{
		RawSocketTool(),
		ExecuteCommandTool(),
		WriteToStdinTool(stdin),
	}
}
