package utils

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/regelepuma/dockerminimizer/logger"
)

var log = logger.Log

func CheckIfFileExists(file string, envPath string) bool {
	_, err := os.Stat(envPath + "/rootfs/" + file)
	return !os.IsNotExist(err)
}

func CheckIfSymbolicLink(file string, envPath string) bool {
	info, err := os.Lstat(envPath + "/rootfs/" + file)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	return false
}

func ReadSymbolicLink(file string, envPath string) string {
	link, _ := os.Readlink(envPath + "/rootfs/" + file)
	resolved := link
	if !filepath.IsAbs(link) {
		resolved = filepath.Join(filepath.Dir(envPath+"/rootfs/"+file), link)
	}
	return strings.TrimPrefix(resolved, envPath+"/rootfs/")
}

func AppendIfMissing[T comparable](slice []T, item T) []T {
	if slices.Contains(slice, item) {
		return slice
	}
	return append(slice, item)
}

func CreateDockerfile(dockerfile string, envPath string, files map[string][]string, symLinks map[string]string) {
	file, _ := os.Create(envPath + "/" + dockerfile)
	defer file.Close()
	srcFile, _ := os.Open(envPath + "/Dockerfile.minimal.template")
	defer srcFile.Close()
	writer := bufio.NewWriter(file)
	_, err := io.Copy(writer, srcFile)
	if err != nil {
		log.Fatalf("Failed to copy template content: %v", err)
	}
	writer.Flush()
	writer.WriteString("\n")
	for dir, libs := range files {
		log.Println("Copying files from " + dir)
		writer.WriteString("COPY --from=builder " + strings.Join(libs, " ") + " " + dir + "\n")
	}
	for link, target := range symLinks {
		log.Println("Copying symbolic link " + link + " to " + target)
		writer.WriteString("COPY --from=builder " + target + " " + link + "\n")
	}
	writer.WriteString("\n")
	writer.Flush()
}

func ValidateDockerfile(dockerfile string, envPath string, context string) error {
	parts := strings.Split(dockerfile, ".")
	tagName := parts[len(parts)-1]
	imageName := "dockerminimize-" + filepath.Base(envPath) + ":" + tagName
	buildPath := envPath + "/" + dockerfile
	_, err := exec.Command("docker", "build", "-f", buildPath, "-t", imageName, context).CombinedOutput()
	log.Info()
	if err != nil {
		log.Error("Failed to build Docker image\n")
		return errors.New("failed to build Docker image")
	}
	_, err = exec.Command("docker", "run", "--rm", imageName).CombinedOutput()
	if err != nil {
		log.Error("Failed to run Docker image\n")
		return errors.New("failed to run Docker image")
	}
	exec.Command("docker", "rmi", imageName).CombinedOutput()
	return nil
}

func Cleanup(envPath string) {
	os.Clearenv()
	err := os.RemoveAll(envPath)
	if err != nil {
		log.Error("Failed to remove temporary files\n")
	}
}
