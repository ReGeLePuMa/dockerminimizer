package strace

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/regelepuma/dockerminimizer/ldd"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func getStraceOutput(imageName string, stracePath string, logPath string, syscalls []string, containerName string, command string, envPath string, metadata types.DockerConfig, timeout int) string {
	hasSudo := utils.HasSudo()
	command = fmt.Sprintf(
		"docker run --rm --name %s --entrypoint \"\" -v %s:/usr/bin/strace -v %s:/log.txt %s /usr/bin/strace -o /log.txt -fe %s %s",
		containerName,
		stracePath,
		logPath,
		imageName,
		strings.Join(syscalls, ","),
		command,
	)
	cmd := exec.Command("sh", "-c", command)
	log.Info("Running command:", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
	data, _ := os.ReadFile(logPath)
	return string(data)
}

func parseOutput(output string, syscalls []string, files map[string][]string, symLinks map[string]string, envPath string) {
	regexes := make(map[string]*regexp.Regexp)
	for _, syscall := range syscalls {
		regexes[syscall] = regexp.MustCompile(syscall + `\([^"]*?"([^"]+)"`)
	}
	for line := range strings.SplitSeq(output, "\n") {
		for _, syscall := range syscalls {
			if regexes[syscall].MatchString(line) {
				match := regexes[syscall].FindStringSubmatch(line)
				if len(match) > 1 {
					utils.AddFilesToDockerfile(match[1], files, symLinks, envPath+"/rootfs")
				}
				break
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
	os.Chmod(envPath+"/strace", 0755)
	_, err = os.Create(envPath + "/log.txt")

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
		log.Error("Failed to find shebang in file:", command)
		return files, symLinks
	}
	match := regex.FindStringSubmatch(shebang)
	if len(match) < 2 {
		log.Error("Failed to find interpreter in shebang:", command)
		return files, symLinks
	}
	interpreter := match[1]
	lddCommand := hasSudo + " chroot " + envPath + "/rootfs ldd " + interpreter
	log.Info("Running command:", lddCommand)
	lddOutput, err := exec.Command("sh", "-c", lddCommand).CombinedOutput()
	if err != nil {
		log.Error("Failed to run ldd command\n" + err.Error())
	}
	files, symLinks = ldd.ParseOutput(lddOutput, envPath+"/rootfs")

	output := getStraceOutput(imageName, envPath+"/strace", envPath+"/log.txt", syscalls,
		containerName, interpreter, envPath, metadata, timeout)
	parseOutput(output, syscalls, files, symLinks, envPath)
	return files, symLinks
}

func DynamicAnalysis(imageName string, envPath string, metadata types.DockerConfig,
	files map[string][]string, symLinks map[string]string, stracePath string, context string, timeout int) error {
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
	log.Info("Creating container:", containerName)
	files, symLinks = parseShebang(imageName, containerName, syscalls, files, symLinks, envPath, metadata, timeout)
	output := getStraceOutput(imageName, envPath+"/strace", envPath+"/log.txt", syscalls,
		containerName, utils.GetFullContainerCommand(imageName, envPath, metadata), envPath, metadata, timeout)
	parseOutput(output, syscalls, files, symLinks, envPath)
	utils.CreateDockerfile("Dockerfile.minimal.strace", "Dockerfile.minimal.template", envPath, files, symLinks)
	log.Info("Validating Dockerfile...")
	return utils.ValidateDockerfile("Dockerfile.minimal.strace", envPath, context, timeout)
}
