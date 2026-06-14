package strace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/regelepuma/dockerminimizer/agent"
	"github.com/regelepuma/dockerminimizer/agent/tools"
	"github.com/regelepuma/dockerminimizer/ldd"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log
var ai_agent agent.Agent
var ctx = context.Background()

const MAX_LIMIT = 127

func getStraceOutput(imageName string, stracePath string, logPath string, syscalls []string, containerName string, command string, envPath string, metadata types.DockerConfig, timeout int) string {
	hasSudo := utils.HasSudo()
	command = fmt.Sprintf(
		"docker run --cap-add=SYS_PTRACE --security-opt seccomp=unconfined --rm --name %s --entrypoint \"\" -v %s:/usr/bin/strace -v %s:/log.txt %s /usr/bin/strace -s 9999 -o /log.txt -fe %s %s",
		containerName,
		stracePath,
		logPath,
		imageName,
		strings.Join(syscalls, ","),
		command,
	)
	cmd := exec.Command("sh", "-c", command)
	if ai_agent != nil {
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			log.Error("Failed to allocate stdin pipe:", err)
			return ""
		}
		defer stdinPipe.Close()
		ai_agent.RegisterTools(tools.GetAvailableTools(stdinPipe))
	}
	log.Debug("Running command:", command)

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	timer := time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		if cmd.Process != nil {
			log.Debug(fmt.Sprintf("%d seconds have passed. Killing strace.", timeout))
			exec.Command("docker", "stop", "-t", "5", containerName).Run()
			utils.ExecCommandWithOptionalSudo(hasSudo, "kill", "-15", fmt.Sprintf("-%d", cmd.Process.Pid)).Run()
		}
	})
	defer timer.Stop()
	err := cmd.Start()

	if err != nil {
		log.Error("Failed to run strace command\n")
		return ""
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	if ai_agent != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()

			prompt := `
				The dynamic analysis has begun.
				Begin interacting with the application after it finishes booting up.
				!!! PLEASE TRY NOT TO CRASH THE APPLICATION !!!
			`

			log.Debug("Starting AI Agent loop in background...")
			_, err := ai_agent.GenerateAction(ctx, prompt)

			if err != nil && err != context.Canceled {
				log.Error("AI Agent finished with error:", err)
			} else {
				log.Debug("AI Agent finished execution.")
			}
		}()
	}

	cmd.Wait()
	cancel()
	wg.Wait()

	data, _ := os.ReadFile(logPath)
	return string(data)
}

func parseOutput(output string, syscalls []string, files map[string][]string, symLinks map[string]string, envPath string) {
	for _, syscall := range syscalls {
		regex := regexp.MustCompile(syscall + `\([^"]*?"([^"]+)"`)
		if regex.MatchString(output) {
			matches := regex.FindAllStringSubmatch(output, -1)
			for _, match := range matches {
				if len(match) > 1 {
					captures := match[1:]
					for _, capture := range captures {
						utils.AddFilesToDockerfile(capture, files, symLinks, envPath+"/rootfs")
					}
				}
			}
		}
	}
}

func prepareEnvironment(envPath string, stracePath string) error {
	err := utils.CopyFile(stracePath, envPath+"/strace")
	if err != nil {
		log.Error("Failed to copy strace to container rootfs")
		return errors.New("failed to copy strace to container rootfs")
	}
	utils.ExecCommandWithOptionalSudo(utils.HasSudo(), "chown", "root:root", envPath+"/strace").Run()
	utils.ExecCommandWithOptionalSudo(utils.HasSudo(), "chmod", "4755", envPath+"/strace").Run()
	_, err = os.Create(envPath + "/log.txt")
	if err != nil {
		log.Error("Failed to create log file: ", envPath+"/log.txt")
		return errors.New("failed to create log file")
	}
	err = os.Chmod(envPath+"/log.txt", 0666)

	return err
}

func getSheBang(command string, rootfsPath string) string {
	file, err := os.Open(rootfsPath + "/" + command)
	if err != nil {
		log.Error("Failed to open file:", command)
		return ""
	}
	defer file.Close()
	var firstLine []byte
	buf := make([]byte, 1)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			firstLine = append(firstLine, buf[0])
		}
		if err != nil {
			break
		}
	}
	return string(firstLine)
}

func parseShebang(imageName string, containerName string, syscalls []string,
	files map[string][]string, symLinks map[string]string, envPath string, metadata types.DockerConfig, timeout int) (map[string][]string, map[string]string) {
	command := utils.GetContainerCommand(imageName, envPath, metadata)
	hasSudo := utils.HasSudo()
	shebang := getSheBang(command, envPath+"/rootfs")
	regex := regexp.MustCompile(`^#!\s*([^\s]+)`)
	if !regex.MatchString(shebang) {
		log.Debug("Failed to find shebang in file:", command)
		return files, symLinks
	}
	match := regex.FindStringSubmatch(shebang)
	if len(match) < 2 {
		log.Error("Failed to find interpreter in shebang:", command)
		return files, symLinks
	}
	interpreter := match[1]
	lddCommand := hasSudo + " chroot " + envPath + "/rootfs ldd " + interpreter
	log.Debug("Running command:", lddCommand)
	lddOutput, err := exec.Command("sh", "-c", lddCommand).CombinedOutput()
	if err != nil {
		log.Error("Failed to run ldd command\n")
	}
	files, symLinks = ldd.ParseOutput(lddOutput, envPath+"/rootfs")

	output := getStraceOutput(imageName, envPath+"/strace", envPath+"/log.txt", syscalls,
		containerName, interpreter, envPath, metadata, timeout)
	parseOutput(output, syscalls, files, symLinks, envPath)
	return files, symLinks
}

func parseCommand(imageName string, containerName string, syscalls []string,
	files map[string][]string, symLinks map[string]string, envPath string, metadata types.DockerConfig, timeout int) (map[string][]string, map[string]string) {
	output := getStraceOutput(imageName, envPath+"/strace", envPath+"/log.txt", syscalls,
		containerName, utils.GetFullContainerCommand(imageName, envPath, metadata), envPath, metadata, timeout)
	parseOutput(output, syscalls, files, symLinks, envPath)
	return files, symLinks
}

func DynamicAnalysis(imageName string, envPath string, metadata types.DockerConfig,
	files map[string][]string, symLinks map[string]string, stracePath string, context string, timeout int, aiType string, aiModel string, aiToken string) error {
	if aiType == "" && aiModel == "" && aiToken == "" {
		log.Info("No AI configuration provided, using standard dynamic analysis approach...")
	} else {

		log.Info("Starting AI-assisted dynamic analysis with AI Type:", aiType, "Model:", aiModel)
	}

	log.Info("Starting dynamic analysis...")
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
	err = prepareEnvironment(envPath, stracePath)
	if err != nil {
		log.Error("Failed to prepare environment for strace")
		log.Error("Skipping dynamic analysis...")
		return errors.New("failed to prepare environment for strace")
	}
	syscalls := []string{
		"open",
		"openat",
		"execve",
		"execveat",
	}
	if files == nil {
		files = make(map[string][]string)
	}
	if symLinks == nil {
		symLinks = make(map[string]string)
	}
	containerName := imageName + "-strace"
	log.Debug("Creating container:", containerName)
	files, symLinks = parseShebang(imageName, containerName, syscalls, files, symLinks, envPath, metadata, timeout)
	files, symLinks = parseCommand(imageName, containerName, syscalls, files, symLinks, envPath, metadata, timeout)
	if len(files)+len(symLinks) > MAX_LIMIT {
		for symlink := range symLinks {
			files[filepath.Dir(symlink)] = utils.AppendIfMissing(files[filepath.Dir(symlink)], symlink)
		}
		tarFilename := fmt.Sprintf("%s/files.tar", envPath)
		if err := utils.BuildTarArchive(files, tarFilename, envPath); err != nil {
			log.Error("Error building tar archive:", err)
			return err
		}
		utils.AddTarToDockerfile("Dockerfile.minimal.strace", "Dockerfile.minimal.ldd", envPath)
		utils.CopyFile(tarFilename, context+"/files.tar")
	} else {
		utils.CreateDockerfile("Dockerfile.minimal.strace", "Dockerfile.minimal.template", envPath, files, symLinks)
	}
	log.Debug("Validating Dockerfile...")
	err = utils.ValidateDockerfile("Dockerfile.minimal.strace", envPath, context, timeout)
	os.Remove(context + "/files.tar")
	return err
}
