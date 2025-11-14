package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/STARRY-S/zip"
)

// Patch patches the executable and all plugins.
func Patch(args Args) error {
	tmpdir, err := os.MkdirTemp(".", ".ipapatch-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	//

	logger.Info("extracting and injecting...")
	paths, err := injectAll(args, tmpdir)
	if err != nil {
		return fmt.Errorf("error injecting: %w", err)
	}

	if args.Output != args.Input {
		logger.Info("copying input to output...")
		if err = copyfile(args.Input, args.Output); err != nil {
			return fmt.Errorf("failed to copy input to output: %w", err)
		}
	}

	var appName string
	if args.UseZip {
		zipArgs := make([]string, 0, len(paths)+2)
		zipArgs = append(zipArgs, "-d", args.Output)
		for _, val := range paths {
			zipArgs = append(zipArgs, val)
		}
		appName = strings.Split(zipArgs[2], "/")[1]
		if err = exec.Command("zip", zipArgs...).Run(); err != nil {
			return fmt.Errorf("error deleting from zipfile: %w", err)
		}
	} else {
		for _, val := range paths {
			appName = strings.Split(val, "/")[1] // yeah i couldnt figure out another way to do this lmao
			break
		}
	}

	//

	logger.Info("adding files back to ipa...")

	o, err := os.OpenFile(args.Output, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer o.Close()

	ud, err := zip.NewUpdater(o)
	if err != nil {
		return err
	}
	defer ud.Close()

	for sysPath, zippedPath := range paths {
		if err = appendFileToUpdater(ud, sysPath, zippedPath); err != nil {
			return err
		}
	}

	if args.Dylib != "" {
		return appendFileToUpdater(ud, args.Dylib, fmt.Sprintf("Payload/%s/Frameworks/%s", appName, filepath.Base(args.Dylib)))
	}

	zxpi, err := zxPluginsInject.Open("resources/zxPluginsInject.dylib")
	if err != nil {
		return err
	}
	defer zxpi.Close()

	return appendToUpdater(
		ud,
		fmt.Sprintf("Payload/%s/Frameworks/zxPluginsInject.dylib", appName),
		zxPluginsInjectInfo{},
		zxpi,
	)
}

func copyfile(from, to string) error {
	f1, err := os.Open(from)
	if err != nil {
		return err
	}
	defer f1.Close()

	f2, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f2.Close()

	_, err = io.Copy(f2, f1)
	return err
}
