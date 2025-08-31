package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/blacktop/go-macho"
	"github.com/blacktop/go-macho/pkg/codesign"
	cstypes "github.com/blacktop/go-macho/pkg/codesign/types"
	"github.com/blacktop/go-macho/types"
)

var ErrNoCodeDirectories = errors.New("no code directories")

var dylibCmdSize = binary.Size(types.DylibCmd{})

func injectLC(fsPath, bundleID, lcName, tmpdir string) error {
	fat, err := macho.OpenFat(fsPath)
	if err == nil {
		defer fat.Close() // in case of returning early

		var slices []string
		for _, arch := range fat.Arches {
			if arch.SubCPU > 2 {
				continue // skip armv7 and other unsupported architectures
			}

			if err = addDylibCommand(arch.File, lcName, bundleID); err != nil {
				return err
			}

			tmp, err := os.CreateTemp(tmpdir, "macho_"+arch.File.CPU.String())
			if err != nil {
				return fmt.Errorf("failed to create temp file: %w", err)
			}
			defer os.Remove(tmp.Name())

			if err = arch.File.Save(tmp.Name()); err != nil {
				return fmt.Errorf("failed to save temp file: %w", err)
			}

			if err = tmp.Close(); err != nil {
				return fmt.Errorf("failed to close temp file: %w", err)
			}

			slices = append(slices, tmp.Name())
		}
		fat.Close()

		// uses os.Create internally, the file will be truncated, everything is fine
		ff, err := macho.CreateFat(fsPath, slices...)
		if err != nil {
			return fmt.Errorf("failed to create fat file: %w", err)
		}
		return ff.Close()
	} else if errors.Is(err, macho.ErrNotFat) {
		m, err := macho.Open(fsPath)
		if err != nil {
			return fmt.Errorf("failed to open MachO file: %w", err)
		}
		defer m.Close()

		if err = addDylibCommand(m, lcName, bundleID); err != nil {
			return err
		}

		// uses WriteFile internally, it also truncates
		if err = m.Save(fsPath); err != nil {
			return fmt.Errorf("failed to save patched MachO file: %w", err)
		}
		return nil
	}
	return err
}

func addDylibCommand(m *macho.File, name, bundleID string) error {
	var cs *macho.CodeSignature
	for i := len(m.Loads) - 1; i >= 0; i-- {
		lc := m.Loads[i]
		cmd := lc.Command()
		if cmd == types.LC_CODE_SIGNATURE {
			m.RemoveLoad(lc)
			cs = lc.(*macho.CodeSignature)
		}
		if cmd != types.LC_LOAD_WEAK_DYLIB && cmd != types.LC_LOAD_DYLIB {
			continue
		}
		if strings.HasPrefix(lc.String(), name) {
			return fmt.Errorf("load command '%s' already exists (already patched)", name)
		}
	}

	var vers types.Version
	vers.Set("0.0.0")

	m.AddLoad(&macho.Dylib{
		DylibCmd: types.DylibCmd{
			LoadCmd:        types.LC_LOAD_WEAK_DYLIB,
			Len:            pointerAlign(uint32(dylibCmdSize + len(name) + 1)),
			NameOffset:     0x18,
			Timestamp:      2, // TODO: I've only seen this value be 2
			CurrentVersion: vers,
			CompatVersion:  vers,
		},
		Name: name,
	})
	if cs != nil {
		if len(cs.CodeDirectories) == 0 {
			return ErrNoCodeDirectories
		}
		cd := cs.CodeDirectories[0]
		if cd.ID == "" {
			cd.ID = bundleID
			if bundleID == "" {
				cd.ID = "fyi.zxcvbn.ipapatch.app" // shouldnt happen, but best to be safe
			}
		}

		// https://github.com/blacktop/go-macho/blob/0247374e8fc354e575b62401a6ec2195d1fae49f/export.go#L265
		return m.CodeSign(&codesign.Config{
			Flags:           cd.Header.Flags | cstypes.ADHOC,
			ID:              cd.ID,
			TeamID:          cd.TeamID,
			Entitlements:    []byte(cs.Entitlements),
			EntitlementsDER: cs.EntitlementsDER,
			SpecialSlots:    []cstypes.SpecialSlot{{Hash: cstypes.EmptySha256Slot}}, // YES this is actually needed
		})
	}
	return nil
}

func pointerAlign(sz uint32) uint32 {
	if (sz % 8) != 0 {
		sz += 8 - (sz % 8)
	}
	return sz
}
