package main

import (
	"fmt"
	"strings"

	"go.uber.org/zap/zapcore"
)

const helpText = `usage: ipapatch [-h/--help] [-i/--input <path>] [-o/--output <path>] [-d/--dylib <path>] [-f/--inplace] [-y/--noconfirm] [-p/--plugins-only] [-z/--zip] [--version]

flags:
  -i, --input path      the path to the ipa file to patch (required)
  -o, --output path     the path to the patched ipa file to create
  -d, --dylib path      the path(s) to dylib(s) to use instead of the embedded zxPluginsInject
                        repeat multiple times: -d tweak1.dylib -d tweak2.dylib
  -f, --inplace         takes priority over --output, use this to overwrite the input file
  -y, --noconfirm       skip interactive confirmation when not using --inplace
  -p, --plugins-only    only inject into plugin binaries (not the main executable)
  -z, --zip             use the zip cli tool to remove files (shouldn't be needed anymore)

info:
  -h, --help            show usage and exit
  --version             show version and exit`

type Args struct {
	Input       string   `arg:"-i,--input,required"`
	Output      string   `arg:"-o,--output"`
	Dylib       []string `arg:"-d,--dylib,separate"`
	InPlace     bool     `arg:"-f,--inplace"`
	NoConfirm   bool     `arg:"-y,--noconfirm"`
	PluginsOnly bool     `arg:"-p,--plugins-only"`
	UseZip      bool     `arg:"-z,--zip"`
}

func (Args) Version() string {
	return "ipapatch v2.1.3"
}

func AskInteractively(question string) bool {
	var reply string
	logger.Infof("%s [Y/n]", question)
	if _, err := fmt.Scanln(&reply); err != nil && err.Error() != "unexpected newline" {
		logger.Logw(zapcore.ErrorLevel, "couldn't scan reply", "err", err)
		return false
	}
	reply = strings.TrimSpace(reply)
	return reply == "" || reply == "y" || reply == "Y"
}
