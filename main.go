package main

import (
	"os"
	"os/exec"
)

func runForIPA(args Args) {
	if args.UseZip {
		if _, err := exec.LookPath("zip"); err != nil {
			logger.Fatal("zip command not found in PATH, you need to install it or omit --zip (see help)")
		}
	}

	// ─────────────────────────────────────────────────────────────
	// Output / inplace resolution
	// ─────────────────────────────────────────────────────────────

	// Default to inplace when no output is provided
	if args.Output == "" {
		args.InPlace = true
		args.Output = args.Input
		logger.Info("--inplace assumed (no --output specified), will overwrite input")
	} else if args.InPlace {
		// Explicit --inplace always wins
		args.Output = args.Input
		logger.Info("--inplace specified, will overwrite input")
	}

	// At this point args.Output is always set.
	// Only protect against overwriting when NOT inplace.
	if !args.InPlace {
		if _, err := os.Stat(args.Output); err == nil {
			if args.NoConfirm {
				logger.Fatal("output file already exists (use --inplace or choose a different --output)")
			}
			if !AskInteractively("output file exists, overwrite?") {
				return
			}
		}
	}

	extractAndInjectIPA(args)
}
