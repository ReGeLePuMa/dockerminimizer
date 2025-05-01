package strace

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/regelepuma/dockerminimizer/logger"
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

func runContainer(imageName string, containerName string) {
	exec.Command("docker", "run", "--rm", "--name", containerName, imageName).Run()
}

func getStraceOutput(containerName string, timeout int) string {
	syscalls := []string{
		"open",
		"openat",
		"execve",
		"execveat",
	}
	hasSudo := utils.HasSudo()
	pid := getPID(containerName)
	log.Info("Container PID:", pid)

	inactivityTimer := time.NewTimer(time.Duration(timeout) * time.Second)

	cmd := exec.Command(hasSudo, "strace", "-p", pid, "-fe", strings.Join(syscalls, ","))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	go func() {
		<-inactivityTimer.C
		log.Info(fmt.Sprintf("%d seconds of inactivity have passed. Killing strace.", timeout))
		if cmd.Process != nil {
			exec.Command(hasSudo, "kill", "-15", fmt.Sprintf("-%d", cmd.Process.Pid)).Run()
			exec.Command(hasSudo, "docker", "kill", containerName).Run()
		}
	}()
	output, _ := cmd.CombinedOutput()

	cmd.Wait()
	return string(output)

}

func DynamicAnalysis(imageName string, envPath string, files map[string][]string, symLinks map[string]string, timeout int) {

	containerName := imageName + "-strace"
	log.Info("Creating container:", containerName)
	var command string
	var wg sync.WaitGroup
	started := make(chan struct{})

	wg.Add(2)

	go func() {
		defer wg.Done()
		close(started)
		command = getStraceOutput(containerName, timeout)
	}()

	go func() {
		defer wg.Done()
		<-started
		runContainer(imageName, containerName)
	}()

	wg.Wait()
	log.Info("Strace output:\n", command)

}
