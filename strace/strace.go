package strace

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func getPID(containerName string) string {
	for {
		pid, err := exec.Command("docker", "inspect", "-f", "{{.State.Pid}}", containerName).Output()
		if err == nil {
			return strings.Trim(string(pid), "\n ")
		}
	}
}

func DynamicAnalysis(imageName string, envPath string, files map[string][]string, symLinks map[string]string) {
	syscalls := []string{
		"open",
		"openat",
		"execve",
		"execveat",
	}
	containerName := imageName + "-strace"
	log.Info("Creating container:", containerName)
	hasSudo := utils.HasSudo()
	exec.Command("docker", "run", "-d", "--rm", "--name", containerName, imageName).Run()
	pid := getPID(containerName)
	log.Info("Container PID:", pid)
	cmd := exec.Command(hasSudo, "strace", "-p", pid, "-fe", strings.Join(syscalls, ","))
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	cmd.Start()
	stream := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(stream)
	inactivityTimer := time.NewTimer(30 * time.Second)
	defer inactivityTimer.Stop()
	var buf bytes.Buffer
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			buf.WriteString(line + "\n")
			inactivityTimer.Reset(30 * time.Second)
		}
	}()

	go func() {
		<-inactivityTimer.C
		log.Println("30 seconds of inactivity detected. Killing strace.")
		if cmd.Process != nil {
			cmd := exec.Command("sh", "-c", "pgrep -f 'sudo strace -p "+pid+"'")
			log.Print(cmd.String())
			out, _ := cmd.Output()
			log.Print(string(out))
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				log.Println("Killing PID: " + line)
				exec.Command(hasSudo, "kill", "-9", line).Run()
			}
			log.Println("Killing container:", containerName)
			exec.Command(hasSudo, "docker", "kill", containerName).Run()
		}
	}()

	cmd.Wait()
	command := buf.String()
	log.Info("Strace output:\n", command)

}
