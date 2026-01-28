package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	// Validate input exists
	if _, err := os.Stat(args.Input); err != nil {
		if os.IsNotExist(err) {
			logger.Fatalf("input path does not exist: %s", args.Input)
		}
		logger.Fatalf("failed to stat input: %v", err)
	}

	// Validate all dylib paths (if any)
	for _, d := range args.Dylib {
		if d == "" {
			continue
		}
		if _, err := os.Stat(d); err != nil {
			if os.IsNotExist(err) {
				logger.Fatalw("path provided to -d/--dylib doesn't exist", "path", d)
			}
			logger.Fatalw("failed to stat dylib", "path", d, "err", err)
		}
	}

	ext := strings.ToLower(filepath.Ext(args.Input))
	switch ext {
	case ".ipa", ".tipa":
		runForIPA(args)
	case ".app":
		runForAppBundle(args)
	default:
		logger.Fatalf("unsupported input type %q (expected .ipa, .tipa, or .app)", ext)
	}
}

func runForIPA(args Args) {
	if args.UseZip {
		if _, err := exec.LookPath("zip"); err != nil {
			logger.Fatal("zip command not found in PATH, you need to install it or omit --zip (see help)")
		}
	}

	// ─────────────────────────────────────────────────────────────
	// Output / inplace resolution
	// ─────────────────────────────────────────────────────────────

	// Default to inplace when no output is specified
	if args.Output == "" {
		args.InPlace = true
		args.Output = args.Input
		logger.Info("--inplace assumed (no --output specified), will overwrite input")
	}

	// Explicit --inplace (kept for compatibility)
	if args.InPlace {
		logger.Info("--inplace specified, will overwrite input")
		args.Output = args.Input
	} else {
		_, err := os.Stat(args.Output)
		if err == nil {
			if args.NoConfirm {
				logger.Info("--output already exists, overwriting")
			} else if !AskInteractively("--output already exists, overwrite?") {
				return
			}
		}
	}

	if err := Patch(args); err != nil {
		logger.Log(zapcore.ErrorLevel, err)
		os.Exit(1)
	}
}

func runForAppBundle(args Args) {
	if args.UseZip {
		logger.Info("--zip has no effect for .app inputs (ignored)")
	}

	if !args.InPlace && args.Output != "" {
		logger.Info("--output is ignored for .app inputs; patching in place")
	}

	if err := PatchAppBundle(args); err != nil {
		logger.Log(zapcore.ErrorLevel, err)
		os.Exit(1)
	}
}
