package ldd

import (
	"bufio"
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func parseOutput(output []byte, envPath string) (map[string][]string, map[string]string) {
	files := make(map[string][]string)
	symLinks := make(map[string]string)
	rootfsPath := envPath + "/rootfs"
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=>") {
			parts := strings.Split(line, "=>")
			lib := strings.Split(strings.TrimSpace(parts[1]), " ")[0]

			if strings.Contains(lib, "not found") {
				continue
			}
			lib = utils.RealPath(lib)
			if utils.CheckIfSymbolicLink(lib, rootfsPath) {
				symLinks[lib] = utils.ReadSymbolicLink(lib, rootfsPath)

			} else if utils.CheckIfFileExists(lib, rootfsPath) {
				files[filepath.Dir(lib)] = utils.AppendIfMissing(files[filepath.Dir(lib)], lib)
			}

		} else if strings.Contains(line, "not found") {
			continue
		} else {
			lib := strings.Split(strings.TrimSpace(line), " ")[0]
			lib = utils.RealPath(lib)
			if utils.CheckIfSymbolicLink(lib, rootfsPath) {
				symLinks[lib] = utils.ReadSymbolicLink(lib, rootfsPath)
			} else if utils.CheckIfFileExists(lib, rootfsPath) {
				files[filepath.Dir(lib)] = utils.AppendIfMissing(files[filepath.Dir(lib)], lib)
			}
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
	libs, symlinkLibs := parseOutput(lddOutput, envPath)
	utils.CreateDockerfile("Dockerfile.minimal.ldd", envPath, command, libs, symlinkLibs)
	log.Info("Validating Dockerfile...")
	return libs, symlinkLibs, utils.ValidateDockerfile("Dockerfile.minimal.ldd", envPath, context, timeout)
}
