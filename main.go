package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/regelepuma/dockerminimizer/ldd"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/preprocess"
	"github.com/regelepuma/dockerminimizer/strace"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

func main() {

	dockerfile := flag.String("file", "./Dockerfile", "Path to the Dockerfile")
	image := flag.String("image", "", "Name of the Docker image")
	flag.Int("max_limit", 10, "Maximum number of retries")
	debug := flag.Bool("debug", false, "Enable debug mode")
	timeout := flag.Int("timeout", 30, "How long should `strace` trace the command")
	flag.Parse()
	if *debug {
		os.Setenv("debug", "true")
	}
	logger.InitLogger()
	log := logger.Log

	args := types.Args{
		Dockerfile: *dockerfile,
		Image:      *image,
	}
	imageName, envPath, metadata := preprocess.ProcessArgs(args)
	files, symLinks, err := ldd.StaticAnalysis(envPath, metadata, filepath.Dir(*dockerfile))
	if err == nil {
		log.Info("Static analysis succeeded")
		log.Info("Cleaning up...")
		utils.Cleanup(envPath)
		return
	}
	log.Error("Static analysis failed, continuing with dynamic analysis")
	strace.DynamicAnalysis(imageName, envPath, files, symLinks, *timeout)
	log.Info("Cleaning up...")
	utils.Cleanup(envPath)

}
