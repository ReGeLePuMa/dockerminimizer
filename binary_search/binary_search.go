package binarysearch

import (
	"crypto/rand"
	"errors"
	"fmt"
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

func binarySearchStep(envPath string, context string, timeout int, step int, usedFiles map[string][]string,
	unusedFiles map[string][]string) (map[string][]string, map[string][]string, error) {
	for {
		if len(unusedFiles) == 0 {
			return nil, nil, errors.New("no unused files or symbolic links left to process")
		}
		usedFiles, unusedFiles = splitFilesystem(usedFiles, unusedFiles)
		filename := fmt.Sprintf("Dockerfile.minimal.binary_search.%d", step)
		tarFilename := fmt.Sprintf("%s/files.tar", envPath)
		if err := utils.BuildTarArchive(usedFiles, tarFilename, envPath); err != nil {
			log.Error("Error building tar archive:", err)
			return nil, nil, err
		}
		err := utils.AddTarToDockerfile(filename, "Dockerfile.minimal.template", envPath)
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
