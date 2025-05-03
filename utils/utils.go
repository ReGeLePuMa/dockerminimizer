package utils

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
)

var log = logger.Log

func RealPath(path string) string {
	realPath, _ := filepath.Abs(path)
	return filepath.Clean(realPath)
}

func CheckIfFileExists(file string, envPath string) bool {
	_, err := os.Stat(envPath + "/" + file)
	return !os.IsNotExist(err)
}

func CheckIfSymbolicLink(file string, envPath string) bool {
	info, err := os.Lstat(envPath + "/" + file)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	return false
}

func ReadSymbolicLink(file string, envPath string) string {
	link, _ := os.Readlink(envPath + "/" + file)
	resolved := link
	if !filepath.IsAbs(link) {
		resolved = filepath.Join(filepath.Dir(envPath+"/"+file), link)
	}
	return strings.TrimPrefix(resolved, envPath+"/")
}

func AppendIfMissing[T comparable](slice []T, item T) []T {
	if slices.Contains(slice, item) {
		return slice
	}
	return append(slice, item)
}

func HasSudo() string {
	if os.Getuid() == 0 {
		return ""
	}
	return "sudo"
}

func copyFile(src string, dest string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	err = destFile.Sync()
	return err
}

func GetFullContainerCommand(metadata types.DockerConfig) string {
	command := ""
	if metadata.Entrypoint != nil {
		command += strings.Join(metadata.Entrypoint, " ")
	} else if metadata.Cmd != nil {
		command += strings.Join(metadata.Cmd, " ")
	}
	return command
}

func GetContainerCommand(imageName string, envPath string, metadata types.DockerConfig) string {
	command := ""
	if metadata.Entrypoint != nil {
		command = filepath.Base(metadata.Entrypoint[0])
	} else if metadata.Cmd != nil {
		command = filepath.Base(metadata.Cmd[0])
	}
	if command == "" {
		log.Error("Failed to find command in Docker image\n")
		Cleanup(envPath, imageName)
		os.Exit(1)
	}
	hasSudo := HasSudo()

	output, err := exec.Command(hasSudo, "chroot", envPath+"/rootfs", "which", command).CombinedOutput()
	log.Info(string(output))
	if err != nil {
		log.Error("Failed to find command in Docker image\n")
		Cleanup(envPath, imageName)
		os.Exit(1)
	}
	return strings.TrimSpace(string(output))
}

func CreateDockerfile(dockerfile string, envPath string, command string, files map[string][]string, symLinks map[string]string) {
	file, _ := os.Create(envPath + "/" + dockerfile)
	defer file.Close()
	srcFile, _ := os.Open(envPath + "/Dockerfile.minimal.template")
	defer srcFile.Close()
	writer := bufio.NewWriter(file)
	_, err := io.Copy(writer, srcFile)
	if err != nil {
		log.Fatalf("Failed to copy template content: %v", err)
	}
	writer.Flush()
	writer.WriteString("\n")
	if command != "" {
		writer.WriteString("COPY --from=builder " + command + " " + command + "\n")
	}
	for dir, libs := range files {
		log.Println("Copying files from " + dir)
		writer.WriteString("COPY --from=builder " + strings.Join(libs, " ") + " " + dir + "/\n")
	}
	for link, target := range symLinks {
		log.Println("Copying symbolic link " + link + " to " + target)
		writer.WriteString("COPY --from=builder " + target + " " + link + "\n")
	}
	writer.WriteString("\n")
	writer.Flush()
}

func ValidateDockerfile(dockerfile string, envPath string, context string, timeout int) error {
	parts := strings.Split(dockerfile, ".")
	tagName := parts[len(parts)-1]
	imageName := "dockerminimize-" + filepath.Base(envPath) + ":" + tagName
	buildPath := envPath + "/" + dockerfile
	output, err := exec.Command("docker", "build", "-f", buildPath, "-t", imageName, context).CombinedOutput()
	log.Info(string(output))
	if err != nil {
		log.Error("Failed to build Docker image\n")
		return errors.New("failed to build Docker image")
	}

	ok := true
	cmd := exec.Command("docker", "run", "--rm", imageName)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	timer := time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		if cmd.Process != nil {
			log.Info(fmt.Sprintf("%d seconds have passed. Killing strace.", timeout))
			exec.Command("docker", "stop", "-t", "5", imageName).Run()
			exec.Command(HasSudo(), "kill", "-15", fmt.Sprintf("-%d", cmd.Process.Pid)).Run()
			ok = false
		}
	})
	defer timer.Stop()
	output, err = cmd.CombinedOutput()

	log.Info(string(output))
	if err != nil || !ok {
		log.Error("Failed to run Docker image\n")
		return errors.New("failed to run Docker image")
	}
	copyFile(envPath+"/"+dockerfile, "Dockerfile.minimal")
	return nil
}

func Cleanup(envPath string, imageName string) {
	command := fmt.Sprintf("docker rmi -f $(docker images %s --format \"{{.Repository}}:{{.Tag}}\")", imageName)
	log.Info("Cleaning up Docker images...")
	log.Info("Running command: " + command)
	exec.Command("sh", "-c", command).CombinedOutput()
	os.Clearenv()
	err := os.RemoveAll(envPath)
	if err != nil {
		log.Error("Failed to remove temporary files\n")
	}
}
