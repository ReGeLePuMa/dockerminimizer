package strace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func getPID(containerName string) string {
	for {
		pid, err := exec.Command("docker", "inspect", "-f", "{{.State.Pid}}", containerName).Output()
		if err == nil {
			return strings.TrimSpace(string(pid))
		}
	}
}

func getStraceOutput(imageName string, containerName string, metadata types.DockerConfig, timeout int) string {
	syscalls := []string{
		"open",
		"openat",
		"execve",
		"execveat",
	}
	command := utils.GetFullContainerCommand(metadata)
	hasSudo := utils.HasSudo()
	command = fmt.Sprintf(
		"docker run --rm --name %s -v /usr/local/bin/strace:/usr/bin/strace %s /usr/bin/strace -fe %s %s",
		containerName,
		imageName,
		strings.Join(syscalls, ","),
		command,
	)
	cmd := exec.Command("sh", "-c", command)
	log.Info("Running command:", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out bytes.Buffer
	cmd.Stderr = &out
	timer := time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		if cmd.Process != nil {
			log.Info(fmt.Sprintf("%d seconds have passed. Killing strace.", timeout))
			exec.Command("docker", "stop", "-t", "5", containerName).Run()
			exec.Command(hasSudo, "kill", "-15", fmt.Sprintf("-%d", cmd.Process.Pid)).Run()
		}
	})
	defer timer.Stop()
	err := cmd.Start()

	if err != nil {
		log.Error("Failed to run strace command\n" + err.Error())
		return ""
	}
	cmd.Wait()
	return out.String()
}

func DynamicAnalysis(imageName string, envPath string, metadata types.DockerConfig, files map[string][]string, symLinks map[string]string, timeout int) error {
	_, err := os.Stat("/usr/local/bin/strace")
	if os.IsNotExist(err) {
		log.Error("Non statically linked strace not found in /usr/local/bin/strace")
		log.Error("Please compile strace statically and copy it to /usr/local/bin/strace")
		log.Error("Skipping dynamic analysis")
		return errors.New("strace not found")
	}

	containerName := imageName + "-strace"
	log.Info("Creating container:", containerName)
	command := getStraceOutput(imageName, containerName, metadata, timeout)

	log.Info("Strace output:\n", command)
	return nil
}
