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
	"github.com/regelepuma/dockerminimizer/utils"
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
	hasSudo := utils.HasSudo()
	cmd = exec.Command(
		hasSudo,
		"docker",
		"build",
		"-f", dockerfile,
		"-o", "type=tar,dest="+envPath+"/rootfs.tar",
		buildContext)
	log.Info(cmd.String())
	output, err = cmd.CombinedOutput()
	log.Info(string(output))
	if err != nil {
		os.RemoveAll(envPath)
		panic("Failed to extract filesystem from Docker image: " + err.Error())
	}
	os.MkdirAll(envPath+"/rootfs", 0777)
	log.Info("Extracting filesystem to:", envPath+"/rootfs")
	exec.Command(hasSudo, "tar", "-xf", envPath+"/rootfs.tar", "-C", envPath+"/rootfs").Run()
	exec.Command(hasSudo, "rm", "-f", envPath+"/rootfs.tar").Run()
	if os.Getuid() != 0 {
		exec.Command("sudo", "chown", "-R", os.Getenv("USER")+":"+os.Getenv("USER"), envPath).Run()
		exec.Command("sudo", "chmod", "-R", "755", envPath).Run()
	}
	return "dockerminimize-" + filepath.Base(envPath)
}

func extractMetadata(imageName string, dockerfile string, envPath string) types.DockerConfig {
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
	for exposedPorts := range config.ExposedPorts {
		writer.WriteString("EXPOSE " + exposedPorts + "\n")
	}
	entrypoints := []string{}
	for _, entrypoint := range config.Entrypoint {
		entrypoints = append(entrypoints, fmt.Sprintf("\"%s\"", entrypoint))
	}
	if len(entrypoints) > 0 {
		writer.WriteString("ENTRYPOINT [" + strings.Join(entrypoints, ", ") + "]\n")
	}
	command := []string{}
	for _, cmd := range config.Cmd {
		command = append(command, fmt.Sprintf("\"%s\"", cmd))
	}
	if len(command) > 0 {
		writer.WriteString("CMD [" + strings.Join(command, ", ") + "]\n")
	}
	writer.Flush()
	return config
}

func parseFile(file string, envPath string, metadata types.DockerConfig,
	files map[string][]string, symLinks map[string]string) {
	workDir := filepath.Clean(envPath + "/rootfs/" + metadata.WorkingDir)
	if utils.CheckIfFileExists(file, workDir) {
		filePath := filepath.Clean(metadata.WorkingDir + "/" + file)
		if utils.CheckIfSymbolicLink(file, workDir) {
			symLinks[filePath] = utils.ReadSymbolicLink(filePath, envPath+"/rootfs")
		} else {
			files[filepath.Dir(filePath)] = utils.AppendIfMissing(files[filepath.Dir(filePath)], filePath)
		}
	} else {
		hasSudo := utils.HasSudo()
		output, err := exec.Command(hasSudo, "chroot", envPath+"/rootfs", "which", file).CombinedOutput()
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(string(output))
		if cmd == "" {
			return
		}
		cmd = filepath.Clean(cmd)
		if !utils.CheckIfFileExists(cmd, envPath+"/rootfs") {
			return
		}
		if utils.CheckIfSymbolicLink(cmd, envPath+"/rootfs") {
			symLinks[cmd] = utils.ReadSymbolicLink(cmd, envPath+"/rootfs")
		} else {
			files[filepath.Dir(cmd)] = utils.AppendIfMissing(files[filepath.Dir(cmd)], cmd)
		}
	}
}

func parseCommand(metadata types.DockerConfig, envPath string) (map[string][]string, map[string]string) {
	files := make(map[string][]string)
	symLinks := make(map[string]string)
	for _, entrypoint := range metadata.Entrypoint {
		parseFile(entrypoint, envPath, metadata, files, symLinks)
	}
	for _, cmd := range metadata.Cmd {
		parseFile(cmd, envPath, metadata, files, symLinks)
	}
	return files, symLinks
}

func processDockerfile(dockerfile string, envPath string, timeout int) (string, string, types.DockerConfig, error) {
	content, _ := os.ReadFile(dockerfile)
	_, err := parser.Parse(strings.NewReader(string(content)))
	if err != nil {
		os.RemoveAll(envPath)
		panic("Failed to parse Dockerfile: " + err.Error())
	}
	imageName := buildAndExtractFilesystem(dockerfile, envPath)
	metadata := extractMetadata(imageName, dockerfile, envPath)
	files, symLinks := parseCommand(metadata, envPath)
	utils.CreateDockerfile("Dockerfile.minimal.initial", "Dockerfile.minimal.template", envPath, files, symLinks)
	err = utils.ValidateDockerfile("Dockerfile.minimal.initial", envPath, filepath.Dir(dockerfile), timeout)
	return imageName, envPath, metadata, err
}

func processImage(imageName string, envPath string, timeout int) (string, string, types.DockerConfig, error) {
	dockerfile, _ := os.Create("Dockerfile")
	defer dockerfile.Close()
	defer os.Remove("Dockerfile")
	writer := bufio.NewWriter(dockerfile)
	writer.WriteString("FROM " + imageName + "\n")
	writer.Flush()
	return processDockerfile("Dockerfile", envPath, timeout)
}

func ProcessArgs(args types.Args) (string, string, types.DockerConfig, error) {
	envPath := createEnvironment()
	if args.Image == "" {
		_, err := os.Stat(args.Dockerfile)
		if os.IsNotExist(err) {
			os.RemoveAll(envPath)
			panic("Dockerfile does not exist")
		}
		return processDockerfile(args.Dockerfile, envPath, args.Timeout)
	}
	return processImage(args.Image, envPath, args.Timeout)
}
