package tools

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

func RawSocketTool() Tool {
	return Tool{
		Name:        "sendRawSocket",
		Description: "Open a raw network socket (TCP or UDP) to a specific port on the container and transmit arbitrary data bytes.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"protocol": map[string]interface{}{
					"type":        "string",
					"description": "The socket protocol to use: 'tcp' or 'udp'",
				},
				"port": map[string]interface{}{
					"type":        "integer",
					"description": "The target container port",
				},
				"encoding": map[string]interface{}{
					"type":        "string",
					"description": "The format of the payload: 'text' for plain strings, 'hex' for hex-encoded binary data (e.g., '414141')",
				},
				"payload": map[string]interface{}{
					"type":        "string",
					"description": "The payload string matching the specified encoding rules.",
				},
			},
			"required": []string{"protocol", "port", "encoding", "payload"},
		},
		Execute: func(containerName string, input map[string]interface{}) (string, error) {
			protocol, _ := input["protocol"].(string)
			encoding, _ := input["encoding"].(string)
			payloadStr, _ := input["payload"].(string)

			portFloat, ok := input["port"].(float64)
			if !ok {
				return "", fmt.Errorf("invalid or missing port")
			}
			port := int(portFloat)

			var rawBytes []byte
			var err error

			switch encoding {
			case "hex":
				rawBytes, err = hex.DecodeString(payloadStr)
				if err != nil {
					return "", fmt.Errorf("failed to decode hex payload: %w", err)
				}
			case "text":
				rawBytes = []byte(payloadStr)
			default:
				return "", fmt.Errorf("unsupported encoding format: %s", encoding)
			}

			containerIP := getContainerIP(containerName)
			targetAddr := fmt.Sprintf("%s:%d", containerIP, port)

			return executeSocketTransfer(protocol, targetAddr, rawBytes)
		},
	}
}

func executeSocketTransfer(protocol, target string, data []byte) (string, error) {
	timeout := 3 * time.Second

	conn, err := net.DialTimeout(protocol, target, timeout)
	if err != nil {
		return "", fmt.Errorf("socket dial connection failed: %w", err)
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	_ = conn.SetReadDeadline(time.Now().Add(timeout))

	_, err = conn.Write(data)
	if err != nil {
		return "", fmt.Errorf("failed writing bytes to socket: %w", err)
	}

	if protocol == "tcp" {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return "Bytes transmitted successfully over TCP. No response returned.", nil
		}
		return fmt.Sprintf("TCP Transmission complete. Response (Hex): %s", hex.EncodeToString(buf[:n])), nil
	}

	return "Bytes transmitted successfully over UDP (blind fire).", nil
}

func getContainerIP(containerName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	formatFlag := "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}"
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", formatFlag, containerName)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "127.0.0.1"
	}

	ip := strings.TrimSpace(stdout.String())
	if ip == "" {
		return "127.0.0.1"
	}

	return ip
}
