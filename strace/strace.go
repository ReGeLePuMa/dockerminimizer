package strace

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func getStraceOutput(imageName string, stracePath string, containerName string, envPath string, metadata types.DockerConfig, timeout int) string {
	syscalls := []string{
		"open",
		"openat",
		"execve",
		"execveat",
	}
	command := utils.GetFullContainerCommand(imageName, envPath, metadata)
	hasSudo := utils.HasSudo()
	command = fmt.Sprintf(
		"docker run --rm --name %s --entrypoint \"\" -v %s:/usr/bin/strace %s /usr/bin/strace -fe %s %s",
		containerName,
		stracePath,
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

func DynamicAnalysis(imageName string, envPath string, metadata types.DockerConfig,
	files map[string][]string, symLinks map[string]string, stracePath string, timeout int) error {
	if !utils.CheckIfFileExists(stracePath, "") {
		log.Error("Strace not found at path:", stracePath)
		log.Error("Skipping dynamic analysis...")
		return errors.New("strace not found")

	}
	_, err := exec.Command("ldd", stracePath).Output()
	if err == nil {
		log.Error("Strace is not statically linked")
		log.Error("Skipping dynamic analysis...")
		return errors.New("strace is not statically linked")
	}

	containerName := imageName + "-strace"
	log.Info("Creating container:", containerName)
	command := getStraceOutput(imageName, stracePath, containerName, envPath, metadata, timeout)

	log.Info("Strace output:\n", command)
	return nil
}
