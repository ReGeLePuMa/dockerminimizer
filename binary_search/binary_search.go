package binarysearch

import (
	"archive/tar"
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"slices"

	"github.com/barkimedes/go-deepcopy"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func parseFilesystem(rootfsPath string) (map[string][]string,
	map[string][]string, error) {
	info, err := os.Stat(rootfsPath)
	if err != nil {
		log.Error("Error reading rootfs path:", err)
		return nil, nil, err
	}
	if !info.IsDir() {
		log.Error("Rootfs path is not a directory")
		return nil, nil, errors.New("rootfs path is not a directory")
	}

	usedFiles := make(map[string][]string)
	unusedFiles := make(map[string][]string)
	err = filepath.WalkDir(rootfsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Error("Error walking directory:", err)
			return err
		}
		relPath := strings.TrimPrefix(path, rootfsPath)
		if relPath == "" {
			relPath = "/"
		}
		unusedFiles[filepath.Dir(relPath)] = utils.AppendIfMissing(unusedFiles[filepath.Dir(relPath)], relPath)
		return nil
	})
	if err != nil {
		log.Error("Error walking directory:", err)
		return nil, nil, err
	}
	return usedFiles, unusedFiles, nil
}

func splitFilesystem(usedFiles map[string][]string,
	unusedFiles map[string][]string) (map[string][]string, map[string][]string) {
	cpyUsedFiles, _ := deepcopy.Anything(usedFiles)
	cpyUnusedFiles, _ := deepcopy.Anything(unusedFiles)
	usedFiles, _ = cpyUsedFiles.(map[string][]string)
	unusedFiles, _ = cpyUnusedFiles.(map[string][]string)
	for dir, files := range unusedFiles {
		originalFiles := slices.Clone(files)
		for _, file := range originalFiles {
			flag, _ := rand.Int(rand.Reader, big.NewInt(2))
			ok := flag.Int64()
			if ok == 0 {
				usedFiles[dir] = utils.AppendIfMissing(usedFiles[dir], file)
				unusedFiles[dir] = utils.RemoveElement(unusedFiles[dir], file)
			}
		}
		if len(unusedFiles[dir]) == 0 {
			delete(unusedFiles, dir)
		}
	}
	return usedFiles, unusedFiles
}

func addFileToTar(tarWriter *tar.Writer, file string, rootfsPath string) error {
	var overrideModes = map[string]int64{
		"/tmp":     01777,
		"tmp":      01777,
		"/var/tmp": 01777,
		"var/tmp":  01777,
		"/root":    0700,
		"root":     0700,
	}
	filePath := rootfsPath + file

	info, err := os.Lstat(filePath)
	if err != nil {
		return err
	}

	var link string
	if info.Mode()&os.ModeSymlink != 0 {
		link, err = os.Readlink(filePath)
		if err != nil {
			return err
		}
	}

	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}

	header.Name = filepath.ToSlash(file)
	header.Uid = 0
	header.Gid = 0
	header.Uname = "root"
	header.Gname = "root"
	if overrideMode, ok := overrideModes[header.Name]; ok {
		header.Mode = overrideMode
	} else {
		header.Mode = int64(info.Mode())
	}
	if info.IsDir() && !strings.HasSuffix(header.Name, "/") {
		header.Name += "/"
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return nil
	}

	fd, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fd.Close()

	_, err = io.Copy(tarWriter, fd)
	return err
}

func buildTarArchive(files map[string][]string, tarFilename string, envPath string) error {
	tarFile, err := os.Create(tarFilename)
	if err != nil {
		log.Error("Failed to create tar file: " + err.Error())
		return err
	}
	defer tarFile.Close()
	tarWriter := tar.NewWriter(tarFile)
	defer tarWriter.Close()
	for _, fileList := range files {
		for _, file := range fileList {
			err := addFileToTar(tarWriter, file, envPath+"/rootfs")
			if err != nil {
				log.Infof("Failed to add %s: %v\n", file, err)
			}
		}
	}
	return nil
}

func addTarToDockerfile(dockerfile string, template string, envPath string) error {
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
	writer.WriteString("ADD files.tar /\n")
	writer.WriteString("\n")
	writer.Flush()
	return nil
}

func binarySearchStep(envPath string, context string, timeout int, step int, usedFiles map[string][]string,
	unusedFiles map[string][]string) (map[string][]string, map[string][]string, error) {
	for {
		if len(unusedFiles) == 0 {
			return nil, nil, errors.New("no unused files or symbolic links left to process")
		}
		usedFiles, unusedFiles = splitFilesystem(usedFiles, unusedFiles)
		filename := fmt.Sprintf("Dockerfile.minimal.binary_search.%d", step)
		tarFilename := fmt.Sprintf("%s/files.tar", envPath)
		if err := buildTarArchive(usedFiles, tarFilename, envPath); err != nil {
			log.Error("Error building tar archive:", err)
			return nil, nil, err
		}
		err := addTarToDockerfile(filename, "Dockerfile.minimal.template", envPath)
		if err != nil {
			log.Error("Error adding tar to Dockerfile:", err)
			return nil, nil, errors.New("error adding tar to Dockerfile")
		}
		utils.CopyFile(tarFilename, context+"/files.tar")
		err = utils.ValidateDockerfile(filename, envPath, context, timeout)
		os.Remove(context + "/files.tar")
		if err != nil {
			exec.Command("docker", "rmi", "-f", "dockerminimize-"+filepath.Base(envPath)+":"+fmt.Sprint(step)).Run()
		}
		if err == nil {
			log.Info("Binary search step ", step, " succeeded.")
			utils.CopyFile(envPath+"/files.tar", "files.tar")
			return make(map[string][]string), usedFiles, nil
		}
	}
}

func BinarySearch(envPath string, maxLimit int, context string, timeout int) error {
	log.Info("Starting binary search...")
	usedFiles, unusedFiles, err := parseFilesystem(envPath + "/rootfs")
	if err != nil {
		log.Error("Error parsing filesystem:", err)
		return errors.New("error parsing filesystem")
	}

	step := 0
	var lastErr error
	for step := 1; step <= maxLimit; step++ {
		log.Info("Binary search iteration:", step)
		usedFiles, unusedFiles, lastErr = binarySearchStep(envPath, context, timeout, step,
			usedFiles, unusedFiles)
		if lastErr != nil {
			break
		}
	}

	if lastErr != nil {
		log.Error("Binary search failed at step", step, "with error:", lastErr)
		return lastErr
	}

	if step > maxLimit {
		log.Info("Reached maximum limit of binary search iterations:", maxLimit)
		return errors.New("reached maximum limit of binary search iterations")
	}

	log.Info("Binary search completed successfully.")
	utils.CopyFile(envPath+"/files.tar", "files.tar")
	return nil
}
