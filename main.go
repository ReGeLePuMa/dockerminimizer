package main

import (
	"flag"
	"os"

	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/preprocess"
	"github.com/regelepuma/dockerminimizer/types"
)

func main() {
	dockerfile := flag.String("file", "./Dockerfile", "Path to the Dockerfile")
	image := flag.String("image", "", "Name of the Docker image")
	retries := flag.Int("max_limit", 10, "Maximum number of retries")
	debug := flag.Bool("debug", false, "Enable debug mode")
	flag.Parse()
	if *debug {
		os.Setenv("debug", "true")
	}
	logger.InitLogger()

	args := types.Args{
		Dockerfile: *dockerfile,
		Image:      *image,
		Retries:    *retries,
	}
	preprocess.ProcessArgs(args)

}
