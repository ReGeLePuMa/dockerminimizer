package main

import (
	"github.com/spf13/cobra"

	"github.com/regelepuma/dockerminimizer"
	"github.com/regelepuma/dockerminimizer/types"
)

func parseArgs(runFunc func(args types.Args)) *cobra.Command {
	var args types.Args
	cmd := &cobra.Command{
		Use:   "dockerminimizer",
		Short: "A tool to minimize Dockerfiles by determining the dependencies of the containerized application",
		Run: func(cmd *cobra.Command, _ []string) {
			runFunc(args)
		},
	}

	cmd.Flags().StringVarP(&args.Dockerfile, "file", "f", "./Dockerfile", "Path to the Dockerfile")
	cmd.Flags().StringVarP(&args.Image, "image", "i", "", "Name of the Docker image")
	cmd.Flags().IntVar(&args.MaxLimit, "max_limit", 10, "Number of binary search steps")
	cmd.Flags().BoolVar(&args.Debug, "debug", false, "Enable debug mode")
	cmd.Flags().IntVar(&args.Timeout, "timeout", 30, "How long the container should run before being declared healthy")
	cmd.Flags().StringVar(&args.StracePath, "strace_path", "/usr/local/bin/strace", "Path to the statically linked strace binary")
	cmd.Flags().BoolVar(&args.BinarySearch, "binary_search", true, "Continue with binary search if dynamic analysis fails")
	return cmd
}

func main() {
	err := parseArgs(dockerminimizer.Run).Execute()
	if err != nil {
		panic(err)
	}
}
