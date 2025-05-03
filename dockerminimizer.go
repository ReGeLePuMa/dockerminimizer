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
	logger.InitLogger()
	log := logger.Log
	log.Info("Starting dockerminimizer...")

	imageName, envPath, metadata := preprocess.ProcessArgs(args)
	files, symLinks, err := ldd.StaticAnalysis(envPath, metadata, filepath.Dir(args.Dockerfile), args.Timeout)
	if err == nil {
		log.Info("Static analysis succeeded")
		log.Info("Cleaning up...")
		utils.Cleanup(envPath)
		return
	}
	log.Error("Static analysis failed, continuing with dynamic analysis")
	strace.DynamicAnalysis(imageName, envPath, metadata, files, symLinks, args.Timeout)
	log.Info("Cleaning up...")
	utils.Cleanup(envPath)
}
