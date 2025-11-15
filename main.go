package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/alexflint/go-arg"
	"go.uber.org/zap/zapcore"
)

//go:embed resources/zxPluginsInject.dylib
var zxPluginsInject embed.FS

func main() {
	var args Args
	if err := arg.Parse(&args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			fmt.Println(helpText)
			return
		} else if errors.Is(err, arg.ErrVersion) {
			fmt.Println(args.Version())
			return
		} else if args.Input == "" {
			fmt.Println(helpText)
			fmt.Println("\nerror: --input is required")
			return
		}
		logger.Fatalf("%v (see --help for usage)", err)
	}

	if args.UseZip {
		if _, err := exec.LookPath("zip"); err != nil {
			logger.Fatal("zip command not found in PATH, you need to install it or omit --zip (see help)")
		}
	}
	if args.InPlace {
		logger.Info("--inplace specified, will overwrite input")
		args.Output = args.Input
	}
	if args.Output == "" {
		if args.NoConfirm {
			logger.Fatal("neither --output nor --inplace specified")
		}
		if !AskInteractively("--inplace not specified, overwrite the input?") {
			return
		}
		args.Output = args.Input
	} else {
		_, err := os.Stat(args.Output)
		if err == nil && !args.InPlace {
			if args.NoConfirm {
				logger.Info("--output already exists, overwriting")
			} else if !AskInteractively("--output already exists, overwrite?") {
				return
			}
		}
	}

	// Multiple --dylib support: validate each path
	if len(args.Dylib) > 0 {
		for _, path := range args.Dylib {
			if path == "" {
				continue
			}
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				logger.Fatalw("path provided to --dylib doesnt exist", "path", path)
			} else if err != nil {
				logger.Fatalw("error checking --dylib path", "path", path, "err", err)
			}
			if info.IsDir() {
				logger.Fatalw("path provided to --dylib is a directory, expected file", "path", path)
			}
		}
	}

	if err := Patch(args); err != nil {
		logger.Log(zapcore.ErrorLevel, err)
		os.Exit(1)
	}
}
