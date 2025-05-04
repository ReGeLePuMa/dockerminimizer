package ldd

import (
	"bufio"
	"bytes"
	"errors"
	"os/exec"
	"strings"

	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func ParseOutput(output []byte, rootfsPath string) (map[string][]string, map[string]string) {
	files := make(map[string][]string)
	symLinks := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=>") {
			parts := strings.Split(line, "=>")
			lib := strings.Split(strings.TrimSpace(parts[1]), " ")[0]

			if strings.Contains(lib, "not found") {
				continue
			}
			utils.AddFilesToDockerfile(lib, files, symLinks, rootfsPath)

		} else if strings.Contains(line, "not found") {
			continue
		} else {
			lib := strings.Split(strings.TrimSpace(line), " ")[0]
			utils.AddFilesToDockerfile(lib, files, symLinks, rootfsPath)
		}
	}
	return files, symLinks
}
func StaticAnalysis(imageName string, envPath string, metadata types.DockerConfig, context string, timeout int) (map[string][]string, map[string]string, error) {
	command := utils.GetContainerCommand(imageName, envPath, metadata)
	hasSudo := utils.HasSudo()
	lddCommand := hasSudo + " chroot " + envPath + "/rootfs ldd " + command
	log.Info("Running command:", lddCommand)
	lddOutput, err := exec.Command("sh", "-c", lddCommand).CombinedOutput()
	if err != nil {
		log.Error("Failed to run ldd command\n" + err.Error())
		return nil, nil, errors.New("failed to run ldd command")
	}
	libs, symlinkLibs := ParseOutput(lddOutput, envPath+"/rootfs")
	utils.CreateDockerfile("Dockerfile.minimal.ldd", "Dockerfile.minimal.initial", envPath, libs, symlinkLibs)
	log.Info("Validating Dockerfile...")
	return libs, symlinkLibs, utils.ValidateDockerfile("Dockerfile.minimal.ldd", envPath, context, timeout)
}
