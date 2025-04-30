package ldd

import (
	"bufio"
	"bytes"
	"errors"
	"os"
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
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=>") {
			parts := strings.Split(line, "=>")
			lib := strings.Split(strings.TrimSpace(parts[1]), " ")[0]

			if strings.Contains(lib, "not found") {
				continue
			}
			if utils.CheckIfSymbolicLink(lib, envPath) {
				symLinks[lib] = utils.ReadSymbolicLink(lib, envPath)

			} else if utils.CheckIfFileExists(lib, envPath) {
				files[filepath.Dir(lib)] = utils.AppendIfMissing(files[filepath.Dir(lib)], lib)
			}

		} else if strings.Contains(line, "not found") {
			continue
		} else {
			lib := strings.Split(strings.TrimSpace(line), " ")[0]

			if utils.CheckIfSymbolicLink(lib, envPath) {
				symLinks[lib] = utils.ReadSymbolicLink(lib, envPath)
			} else if utils.CheckIfFileExists(lib, envPath) {
				files[filepath.Dir(lib)] = utils.AppendIfMissing(files[filepath.Dir(lib)], lib)
			}
		}
	}
	return files, symLinks
}
func StaticAnalysis(envPath string, metadata types.DockerConfig, context string) error {
	command := ""
	if metadata.Entrypoint != nil {
		command = filepath.Base(metadata.Entrypoint[0])
	} else if metadata.Cmd != nil {
		command = filepath.Base(metadata.Cmd[0])
	}
	if command == "" {
		log.Info("No command found in Dockerfile")
		return errors.New("no command found in Dockerfile")
	}
	log.Info("Command to analyze:", command)
	var hasSudo string
	if os.Getuid() == 0 {
		hasSudo = ""
	} else {
		hasSudo = "sudo"
	}
	lddCommand := hasSudo + " chroot " + envPath + "/rootfs ldd $(" +
		hasSudo + " chroot " + envPath + "/rootfs which " + command + ")"
	log.Info("Running command:", lddCommand)
	lddOutput, err := exec.Command("sh", "-c", lddCommand).CombinedOutput()
	if err != nil {
		log.Error("Failed to run ldd command\n" + err.Error())
		return errors.New("failed to run ldd command")
	}
	libs, symlinkLibs := parseOutput(lddOutput, envPath)
	utils.CreateDockerfile("Dockerfile.minimal.ldd", envPath, libs, symlinkLibs)
	log.Info("Validating Dockerfile")
	return utils.ValidateDockerfile("Dockerfile.minimal.ldd", envPath, context)
}
