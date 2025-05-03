package dockerminimizer

import (
	"os"
	"path/filepath"

	"github.com/regelepuma/dockerminimizer/ldd"
	"github.com/regelepuma/dockerminimizer/logger"
	"github.com/regelepuma/dockerminimizer/preprocess"
	"github.com/regelepuma/dockerminimizer/strace"
	"github.com/regelepuma/dockerminimizer/types"
	"github.com/regelepuma/dockerminimizer/utils"
)

func Run(args types.Args) {
	if args.Dockerfile == "" {
		args.Dockerfile = "./Dockerfile"
	}
	if args.MaxLimit == 0 {
		args.MaxLimit = 10
	}
	if args.Timeout == 0 {
		args.Timeout = 30
	}
	if args.Debug {
		os.Setenv("debug", "true")
	}
	if args.StracePath == "" {
		args.StracePath = "/usr/local/bin/strace"
	}
	logger.InitLogger()
	log := logger.Log
	log.Info("Starting dockerminimizer...")

	imageName, envPath, metadata, err := preprocess.ProcessArgs(args)
	if err == nil {
		log.Error("Dockerfile is already minimal")
		log.Info("Cleaning up...")
		utils.Cleanup(envPath, imageName)
		return
	}
	log.Info("Dockerfile is not minimal, starting analysis...")
	files, symLinks, err := ldd.StaticAnalysis(imageName, envPath, metadata, filepath.Dir(args.Dockerfile), args.Timeout)
	if err == nil {
		log.Info("Static analysis succeeded")
		log.Info("Cleaning up...")
		utils.Cleanup(envPath, imageName)
		return
	}
	log.Error("Static analysis failed, continuing with dynamic analysis")
	strace.DynamicAnalysis(imageName, envPath, metadata, files, symLinks, args.StracePath, args.Timeout)
	log.Info("Cleaning up...")
	utils.Cleanup(envPath, imageName)
}
