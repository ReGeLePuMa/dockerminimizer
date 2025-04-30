package preprocess

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
)

var log = logger.Log

func createEnvironment() string {
	dir := md5.Sum(fmt.Appendf(nil, "%d", time.Now().UnixNano()))
	dirStr := hex.EncodeToString(dir[:])
	homeDir, _ := os.UserHomeDir()
	err := os.MkdirAll(homeDir+"/.dockerminimizer/"+dirStr, 0777)
	if err != nil {
		panic("Failed to create directory: " + err.Error())
	}
	log.Info("Created directory:", homeDir+"/.dockerminimizer/"+dirStr)
	return (homeDir + "/.dockerminimizer/" + dirStr)
}

func buildAndExtractFilesystem(dockerfile string, envPath string) string {
	buildContext := filepath.Dir(dockerfile)
	cmd := exec.Command("docker", "build", "-f", dockerfile, "-t", "dockerminimize-"+filepath.Base(envPath), buildContext)
	log.Info(cmd.String())
	output, err := cmd.CombinedOutput()
	log.Info(string(output))
	if err != nil {
		os.RemoveAll(envPath)
		panic("Failed to build Docker image: " + err.Error())
	}
	var hasSudo string
	if os.Getuid() == 0 {
		hasSudo = ""
	} else {
		hasSudo = "sudo"
	}
	cmd = exec.Command(hasSudo, "docker", "build", "-f", dockerfile, "-o", envPath+"/rootfs", buildContext)
	log.Info(cmd.String())
	output, err = cmd.CombinedOutput()
	log.Info(string(output))
	if err != nil {
		os.RemoveAll(envPath)
		panic("Failed to extract filesystem from Docker image: " + err.Error())
	}
	if os.Getuid() != 0 {
		exec.Command("sudo", "chown", "-R", os.Getenv("USER")+":"+os.Getenv("USER"), envPath).Run()
		exec.Command("sudo", "chmod", "-R", "755", envPath).Run()
	}
	return "dockerminimize-" + filepath.Base(envPath)
}

func extractMetadata(imageName string, dockerfile string, envPath string) {
	fd, _ := os.Open(dockerfile)
	defer fd.Close()
	scanner := bufio.NewScanner(fd)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "FROM") {
			lines[i] = lines[i] + " as builder"
			break
		}
	}
	cmd := exec.Command("docker", "inspect", "--format", "{{json .Config}}", imageName)
	log.Info(cmd.String())
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		os.RemoveAll(envPath)
		panic("Failed to inspect Docker image: " + err.Error())
	}
	var config types.DockerConfig
	err = json.Unmarshal(out.Bytes(), &config)
	if err != nil {
		os.RemoveAll(envPath)
		panic("Failed to unmarshal Docker config: " + err.Error())
	}
	file, _ := os.Create(envPath + "/Dockerfile.minimal.template")
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, line := range lines {
		writer.WriteString(line + "\n")
	}
	writer.WriteString("\n\n" + "FROM scratch\n\n")
	for _, env := range config.Env {
		writer.WriteString("ENV " + env + "\n")
	}
	if config.WorkingDir != "" {
		writer.WriteString("WORKDIR " + config.WorkingDir + "\n")
	}
	if config.User != "" {
		writer.WriteString("USER " + config.User + "\n")
	}
	for _, exposedPorts := range config.ExposedPorts {
		for port := range exposedPorts {
			writer.WriteString("EXPOSE " + port + "\n")
		}
	}
	for _, entrypoint := range config.Entrypoint {
		writer.WriteString("ENTRYPOINT [\"" + entrypoint + "\"]\n")
	}
	for _, cmd := range config.Cmd {
		writer.WriteString("CMD [\"" + cmd + "\"]\n")
	}
	writer.Flush()
}

func processDockerfile(dockerfile string, envPath string) (string, string) {
	content, _ := os.ReadFile(dockerfile)
	_, err := parser.Parse(strings.NewReader(string(content)))
	if err != nil {
		os.RemoveAll(envPath)
		panic("Failed to parse Dockerfile: " + err.Error())
	}
	imageName := buildAndExtractFilesystem(dockerfile, envPath)
	extractMetadata(imageName, dockerfile, envPath)
	return imageName, envPath
}

func processImage(imageName string, envPath string) (string, string) {
	dockerfile, _ := os.Create("Dockerfile")
	defer dockerfile.Close()
	defer os.Remove("Dockerfile")
	writer := bufio.NewWriter(dockerfile)
	writer.WriteString("FROM " + imageName + "\n")
	writer.Flush()
	return processDockerfile("Dockerfile", envPath)
}

func ProcessArgs(args types.Args) (string, string) {
	envPath := createEnvironment()
	if args.Image == "" {
		_, err := os.Stat(args.Dockerfile)
		if os.IsNotExist(err) {
			os.RemoveAll(envPath)
			panic("Dockerfile does not exist")
		}
		return processDockerfile(args.Dockerfile, envPath)
	}
	return processImage(args.Image, envPath)
}
