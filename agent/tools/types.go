package tools

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Execute     func(containerName string, input map[string]interface{}) (string, error)
}
