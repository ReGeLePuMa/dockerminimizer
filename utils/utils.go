package utils

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/barkimedes/go-deepcopy"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/samber/lo"
)

var log = logger.Log

func RealPath(path string) string {
	realPath, _ := filepath.Abs(path)
	return filepath.Clean(realPath)
}

func CheckIfDirectoryExists(dir string, envPath string) bool {
	info, err := os.Stat(envPath + "/" + dir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func CheckIfFileExists(file string, envPath string) bool {
	info, err := os.Stat(envPath + "/" + file)
	return !os.IsNotExist(err) && !info.IsDir()
}

func CheckIfSymbolicLink(file string, envPath string) bool {
	info, err := os.Lstat(envPath + "/" + file)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func ReadSymbolicLink(file string, envPath string) string {
	link, _ := os.Readlink(envPath + "/" + file)
	resolved := link
	if !filepath.IsAbs(link) {
		resolved = filepath.Join(filepath.Dir(envPath+"/"+file), link)
	}
	return strings.TrimPrefix(resolved, envPath)
}

func RemoveElement[T comparable](slice []T, item T) []T {
	index := slices.Index(slice, item)
	if index == -1 {
		return slice
	}
	return slices.Delete(slice, index, index+1)
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

func CopyFile(src string, dest string) error {
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

func GetFullContainerCommand(imageName string, envPath string, metadata types.DockerConfig) string {
	command := GetContainerCommand(imageName, envPath, metadata) + " "
	if len(metadata.Entrypoint) > 1 {
		command += strings.Join(metadata.Entrypoint[1:], " ")
	}
	command += " "
	if len(metadata.Cmd) > 0 {
		var cmd []string
		if len(metadata.Entrypoint) > 0 {
			cmd = metadata.Cmd
		} else {
			cmd = metadata.Cmd[1:]
		}
		command += strings.Join(cmd, " ")
	}
	return strings.TrimSpace(command)
}

func GetContainerCommand(imageName string, envPath string, metadata types.DockerConfig) string {
	command := ""
	if len(metadata.Entrypoint) > 0 {
		command = metadata.Entrypoint[0]
	} else if len(metadata.Cmd) > 0 {
		command = metadata.Cmd[0]
	}
	if command == "" {
		log.Error("Failed to find command in Docker image\n")
		Cleanup(envPath, imageName)
		os.Exit(1)
	}
	if filepath.IsAbs(command) {
		return command
	}
	command = filepath.Base(command)
	hasSudo := HasSudo()

	output, err := exec.Command(hasSudo, "chroot", envPath+"/rootfs", "which", command).CombinedOutput()
	cmd := strings.TrimSpace(string(output))
	if err != nil {
		cmd = metadata.WorkingDir + "/" + command
		if !CheckIfFileExists(filepath.Clean(cmd), envPath+"/rootfs") {
			log.Error("Failed to find command in filesystem")

			Cleanup(envPath, imageName)
			os.Exit(1)
		}
	}
	log.Info("Command found: " + cmd)
	return cmd
}

func iterToSlice[T any](s iter.Seq[T]) []T {
	out := []T{}
	for v := range s {
		out = append(out, v)
	}
	return out
}

func lowestCommonAncestor(s1, s2 string) string {
	seg1 := lo.Filter(strings.Split(s1, "/"), func(seg string, _ int) bool {
		return seg != ""
	})
	seg2 := lo.Filter(strings.Split(s2, "/"), func(seg string, _ int) bool {
		return seg != ""
	})

	minLen := min(len(seg1), len(seg2))
	i := 0
	for i < minLen && seg1[i] == seg2[i] {
		i++
	}
	if i == 0 {
		return "/"
	}
	return filepath.Clean("/" + strings.Join(seg1[:i], "/"))
}

func ShrinkDictionary(dict map[string][]string, envPath string) map[string][]string {
	const MAX_LIMIT = 127
	cpyDict, _ := deepcopy.Anything(dict)
	dict, _ = cpyDict.(map[string][]string)
	for len(dict) > MAX_LIMIT {
		keys := iterToSlice(maps.Keys(dict))
		keys = lo.Filter(keys, func(key string, _ int) bool {
			return len(key) > 0
		})
		slices.SortFunc(keys, func(a, b string) int {
			return strings.Compare(a, b)
		})
		for i := 0; i < len(keys)-1; i += 2 {
			delete(dict, keys[i])
			delete(dict, keys[i+1])
			ancestor := lowestCommonAncestor(keys[i], keys[i+1])
			files, ok := dict[ancestor]
			newFiles := []string{}
			if filepath.Clean(keys[i]) != filepath.Clean(ancestor) {
				name := keys[i]
				if CheckIfDirectoryExists(name, envPath+"/rootfs") {
					name = name + "/"
				}
				newFiles = append(newFiles, name)
			}
			if filepath.Clean(keys[i+1]) != filepath.Clean(ancestor) {
				name := keys[i+1]
				if CheckIfDirectoryExists(name, envPath+"/rootfs") {
					name = name + "/"
				}
				newFiles = append(newFiles, name)
			}
			if !ok {
				if len(newFiles) > 0 {
					dict[ancestor] = newFiles
				}
			} else {
				files = lo.Reduce(newFiles, func(files []string, file string, _ int) []string {
					return AppendIfMissing(files, file)
				}, files)
				copyFiles := []string{}
				for i := range files {
					for j := i + 1; j < len(files); j++ {
						combined := lowestCommonAncestor(files[i], files[j])
						copyFiles = AppendIfMissing(copyFiles, combined)
					}
				}
				if len(copyFiles) > 1 {
					dict[ancestor] = copyFiles
				}
			}
		}
	}
	return dict
}

func AddFilesToDockerfile(file string, files map[string][]string, symLinks map[string]string, rootfsPath string) {
	file = RealPath(file)
	if CheckIfFileExists(file, rootfsPath) {
		if CheckIfSymbolicLink(file, rootfsPath) {
			symLinks[file] = ReadSymbolicLink(file, rootfsPath)
		} else {
			files[filepath.Dir(file)] = AppendIfMissing(files[filepath.Dir(file)], file)
		}
	}
}

func CreateDockerfile(dockerfile string, template string, envPath string, files map[string][]string, symLinks map[string]string) {
	file, _ := os.Create(envPath + "/" + dockerfile)
	defer file.Close()
	srcFile, _ := os.Open(envPath + "/" + template)
	defer srcFile.Close()
	writer := bufio.NewWriter(file)
	_, err := io.Copy(writer, srcFile)
	if err != nil {
		log.Fatalf("Failed to copy template content: %v", err)
	}
	writer.Flush()
	writer.WriteString("\n")
	for dir, file := range files {
		quoted := make([]string, len(file))
		for i, s := range file {
			quoted[i] = fmt.Sprintf("\"%s\"", s)
		}
		quoted = append(quoted, filepath.Clean(fmt.Sprintf("\"%s/\"", dir)))
		quoted = slices.Compact(quoted)
		log.Println("Copying files from " + dir)
		writer.WriteString("COPY --from=builder [" + strings.Join(quoted, ", ") + "]\n")
	}
	for link, target := range symLinks {
		log.Println("Copying symbolic link " + link + " to " + target)
		writer.WriteString("COPY --from=builder [\"" + target + "\",  \"" + link + "\"]\n")
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
	containerName := strings.ReplaceAll(imageName, ":", "-") + "-test-" + tagName
	cmd := exec.Command("docker", "run", "--rm", "--name", containerName, imageName)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	timer := time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		if cmd.Process != nil {
			log.Info(fmt.Sprintf("%d seconds have passed. Stopping docker.", timeout))
			exec.Command("docker", "stop", "-t", "5", containerName).Run()
			exec.Command(HasSudo(), "kill", "-15", fmt.Sprintf("-%d", cmd.Process.Pid)).Run()
			ok = false
		}
	})
	defer timer.Stop()
	output, err = cmd.CombinedOutput()

	log.Info(string(output))
	if ok && err != nil {
		log.Error("Failed to run Docker image\n")
		return errors.New("failed to run Docker image")
	}
	CopyFile(envPath+"/"+dockerfile, "Dockerfile.minimal")
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
