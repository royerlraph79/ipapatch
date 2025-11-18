package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/STARRY-S/zip"
	"howett.net/plist"
)

type PlistInfo struct {
	Executable string `plist:"CFBundleExecutable"`
	BundleID   string `plist:"CFBundleIdentifier"`
}

var (
	ErrNoPlist   = errors.New("no Info.plist found in ipa")
	ErrNoPlugins = errors.New("no plugins found")
)

// key - path to file in provided tmpdir, now patched
// val - path inside ipa
func injectAll(args Args, tmpdir string) (map[string]string, error) {
	z, err := zip.OpenReader(args.Input)
	if err != nil {
		return nil, err
	}
	defer z.Close()

	plists, err := findPlists(z.File, args.PluginsOnly)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]string, len(plists))

	// Build list of LC_LOAD_* names to inject (IPA path)
	var lcNames []string
	if len(args.Dylib) == 0 {
		// No custom dylib provided: use embedded zxPluginsInject
		lcNames = []string{"@rpath/zxPluginsInject.dylib"}
	} else {
		seen := make(map[string]struct{})
		for _, dylibPath := range args.Dylib {
			if dylibPath == "" {
				continue
			}
			name := "@rpath/" + filepath.Base(dylibPath)
			if _, ok := seen[name]; ok {
				continue // avoid duplicate LC_LOAD entries
			}
			seen[name] = struct{}{}
			lcNames = append(lcNames, name)
		}
	}

	for _, p := range plists {
		pl, err := getExecutableNames(z, p)
		if err != nil {
			return nil, err
		}

		pathInIpa := path.Join(path.Dir(p), pl.Executable)
		fsPath, err := extractToPath(z, tmpdir, pathInIpa)
		if err != nil {
			return nil, fmt.Errorf("error extracting %s: %w", pl.Executable, err)
		}

		logger.Infof("injecting into %s...", pl.Executable)

		// Inject all desired LC_LOAD_* entries into this binary
		for _, lcName := range lcNames {
			if err = injectLC(fsPath, pl.BundleID, lcName, tmpdir); err != nil {
				return nil, fmt.Errorf("couldn't inject '%s' into %s: %w", lcName, pl.Executable, err)
			}
		}

		paths[fsPath] = pathInIpa
	}

	return paths, nil
}

func findPlists(files []*zip.File, pluginsOnly bool) (plists []string, err error) {
	plists = make([]string, 0, 10)

	for _, f := range files {
		if strings.Contains(f.Name, ".app/Watch") || strings.Contains(f.Name, ".app/WatchKit") || strings.Contains(f.Name, ".app/com.apple.WatchPlaceholder") {
			logger.Infof("found watch app at '%s', you might want to remove that", filepath.Dir(f.Name))
			continue
		}
		if strings.HasSuffix(f.Name, ".appex/Info.plist") {
			plists = append(plists, f.Name)
			continue
		}
		if !pluginsOnly && strings.HasSuffix(f.Name, ".app/Info.plist") {
			plists = append(plists, f.Name)
			continue
		}
	}

	if len(plists) == 0 {
		if pluginsOnly {
			return nil, ErrNoPlugins
		}
		return nil, ErrNoPlist
	}
	return plists, nil
}

func getExecutableNames(z *zip.ReadCloser, plistName string) (*PlistInfo, error) {
	f, err := z.Open(plistName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	contents, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var pl PlistInfo
	_, err = plist.Unmarshal(contents, &pl)
	return &pl, err
}

func extractToPath(z *zip.ReadCloser, dir, name string) (string, error) {
	f, err := z.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()

	output := filepath.Join(dir, filepath.Base(name))
	ff, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		return "", err
	}
	defer ff.Close()

	_, err = io.Copy(ff, f)
	return output, err
}

func appendFileToUpdater(ud *zip.Updater, path, zippedPath string) error {
	o, err := os.Open(path)
	if err != nil {
		return err
	}
	defer o.Close()

	fi, err := o.Stat()
	if err != nil {
		return err
	}

	return appendToUpdater(ud, zippedPath, fi, o)
}

func appendToUpdater(ud *zip.Updater, zippedPath string, fi fs.FileInfo, r io.Reader) error {
	hdr, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}

	hdr.Name = zippedPath
	hdr.Method = zip.Deflate

	w, err := ud.AppendHeader(hdr, zip.APPEND_MODE_OVERWRITE)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, r)
	return err
}

// PatchAppBundle patches an iOS .app bundle on disk (e.g. Payload/Instagram.app),
// mirroring IPA behavior: if no -d is supplied, it injects the embedded
// zxPluginsInject.dylib; otherwise it uses the provided dylib(s).
func PatchAppBundle(args Args) error {
	appPath := args.Input

	// iOS-style: MyApp.app/Info.plist
	infoPath := filepath.Join(appPath, "Info.plist")
	if _, err := os.Stat(infoPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("could not find Info.plist in %s", appPath)
		}
		return fmt.Errorf("failed to stat Info.plist: %w", err)
	}

	contents, err := os.ReadFile(infoPath)
	if err != nil {
		return fmt.Errorf("failed to read Info.plist: %w", err)
	}

	var pl PlistInfo
	if _, err := plist.Unmarshal(contents, &pl); err != nil {
		return fmt.Errorf("failed to parse Info.plist: %w", err)
	}

	// iOS on disk / Payload-style: MyApp.app/<CFBundleExecutable>
	binPath := filepath.Join(appPath, pl.Executable)
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("executable not found at %s: %w", binPath, err)
	}

	// Build list of LC_LOAD_* names to inject (same behavior as IPA)
	var lcNames []string
	if len(args.Dylib) == 0 {
		// No custom dylib: use embedded zxPluginsInject
		lcNames = []string{"@rpath/zxPluginsInject.dylib"}
	} else {
		seen := make(map[string]struct{})
		for _, dylibPath := range args.Dylib {
			if dylibPath == "" {
				continue
			}
			name := "@rpath/" + filepath.Base(dylibPath)
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			lcNames = append(lcNames, name)
		}
	}

	// Temporary dir for fat-file rewrites
	tmpdir, err := os.MkdirTemp("", ".ipapatch-app-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	logger.Infof("injecting into %s...", binPath)
	for _, lcName := range lcNames {
		if err := injectLC(binPath, pl.BundleID, lcName, tmpdir); err != nil {
			return fmt.Errorf("couldn't inject '%s' into %s: %w", lcName, binPath, err)
		}
	}

	// Copy dylibs into the bundle's Frameworks folder (iOS layout)
	frameworksDir := filepath.Join(appPath, "Frameworks")
	if err := os.MkdirAll(frameworksDir, 0755); err != nil {
		return fmt.Errorf("failed to create Frameworks dir: %w", err)
	}

	if len(args.Dylib) == 0 {
		// No custom dylib: copy embedded zxPluginsInject into Frameworks
		zxpi, err := zxPluginsInject.Open("resources/zxPluginsInject.dylib")
		if err != nil {
			return fmt.Errorf("failed to open embedded zxPluginsInject.dylib: %w", err)
		}
		defer zxpi.Close()

		dst := filepath.Join(frameworksDir, "zxPluginsInject.dylib")
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", dst, err)
		}
		if _, err := io.Copy(out, zxpi); err != nil {
			out.Close()
			return fmt.Errorf("failed to write %s: %w", dst, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("failed to close %s: %w", dst, err)
		}
	} else {
		// Custom dylibs: copy all of them into Frameworks
		for _, dylibPath := range args.Dylib {
			if dylibPath == "" {
				continue
			}
			dst := filepath.Join(frameworksDir, filepath.Base(dylibPath))
			if err := copyfile(dylibPath, dst); err != nil {
				return fmt.Errorf("failed to copy %s -> %s: %w", dylibPath, dst, err)
			}
		}
	}

	return nil
}
