package main

import (
	"flag"

	"github.com/regelepuma/dockerminimizer"
	"github.com/regelepuma/dockerminimizer/types"
)

func parseArgs() types.Args {
	dockerfile := flag.String("file", "./Dockerfile", "Path to the Dockerfile")
	image := flag.String("image", "", "Name of the Docker image")
	maxLimit := flag.Int("max_limit", 10, "Maximum number of retries")
	debug := flag.Bool("debug", false, "Enable debug mode")
	timeout := flag.Int("timeout", 30, "How long the container should run before being declared healthy")
	stracePath := flag.String("strace_path", "/usr/local/bin/strace", "Path to the statically linked strace binary")
	binarySearch := flag.Bool("binary_search", true, "Continue with binary search if dynamic analysis fails")

	flag.Parse()

	return types.Args{
		Dockerfile:   *dockerfile,
		Image:        *image,
		MaxLimit:     *maxLimit,
		Debug:        *debug,
		Timeout:      *timeout,
		StracePath:   *stracePath,
		BinarySearch: *binarySearch,
	}
}

func main() {
	dockerminimizer.Run(parseArgs())
}
