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
	"path/filepath"
	"strings"

	"slices"

	"github.com/barkimedes/go-deepcopy"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/utils"
)

var log = logger.Log

func addFileToTar(tarWriter *tar.Writer, file string, envPath string) error {
	filePath := envPath + "/rootfs" + file
	fd, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fd.Close()
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = file
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tarWriter, fd)
	return err
}

func buildTarArchive(files map[string][]string, tarFilename string, envPath string, step int) error {
	tarFile, err := os.Create(tarFilename)
	if err != nil {
		log.Error("Failed to create tar file: " + err.Error())
		return err
	}
	defer tarFile.Close()
	tarWriter := tar.NewWriter(tarFile)
	for _, fileList := range files {
		for _, file := range fileList {
			err := addFileToTar(tarWriter, file, envPath)
			if err != nil {
				log.Info("Failed to add %s: %v\n", file, err)
			}
		}
	}
	return nil
}

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
		if d.IsDir() {
			return nil
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

func addTarToDockerfile(tarFilename string, dockerfile string, template string, envPath string) error {
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
	writer.WriteString(fmt.Sprintf("ADD  %s /\n", tarFilename))
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
		tarFilename := fmt.Sprintf("%s/rootfs/files-%d.tar", envPath, step)
		buildTarArchive(usedFiles, tarFilename, envPath, step)
		err := addTarToDockerfile(tarFilename, filename, "Dockerfile.minimal.template", envPath)
		if err != nil {
			log.Error("Error adding tar to Dockerfile:", err)
			return nil, nil, errors.New("error adding tar to Dockerfile")
		}
		err = utils.ValidateDockerfile(filename, envPath, context, timeout)
		if err == nil {
			log.Info("Binary search step", step, "succeeded.")
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
	step := 1
	for i := range maxLimit {
		log.Info("Binary search iteration:", step)
		usedFiles, unusedFiles, err = binarySearchStep(envPath, context, timeout, step,
			usedFiles, unusedFiles)
		if err != nil {
			break
		}
		step = i + 1
	}
	if step == maxLimit {
		log.Info("Reached maximum limit of binary search iterations:", maxLimit)
		return errors.New("reached maximum limit of binary search iterations")
	}
	log.Info("Binary search completed sucessfully.")
	return nil
}
