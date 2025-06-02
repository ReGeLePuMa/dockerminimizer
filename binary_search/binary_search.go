package binarysearch

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"slices"

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
	for dir, files := range unusedFiles {
		originalFiles := slices.Clone(files)
		for _, file := range originalFiles {
			ok := rand.Int() % 2
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

func binarySearchStep(envPath string, context string, timeout int, step int, usedFiles map[string][]string,
	unusedFiles map[string][]string) (map[string][]string, map[string][]string, error) {
	for {
		if len(unusedFiles) == 0 {
			return nil, nil, errors.New("no unused files or symbolic links left to process")
		}
		usedFiles, unusedFiles = splitFilesystem(usedFiles, unusedFiles)
		filename := fmt.Sprintf("Dockerfile.minimal.binary_search.%d", step)
		usedFiles = utils.ShrinkDictionary(usedFiles)
		utils.CreateDockerfile(filename, "Dockerfile.minimal.template", envPath, usedFiles, nil)
		err := utils.ValidateDockerfile(filename, envPath, context, timeout)
		if err == nil {
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
