package main

import (
	"flag"
	"os"
)

const helpText = `
Usage:
  ipapatch -i <input.ipa> [options]

Options:
  -i, --input <file>     input IPA file (required)
  -o, --output <file>    write output to file (default: overwrite input)
  -f, --inplace          overwrite input file (implicit if --output is not set)
  -y, --yes              assume yes to all prompts
  --zip                  use external zip command
  -h, --help             show this help message
`

func parseArgs() Args {
	var args Args

	flag.StringVar(&args.Input, "i", "", "")
	flag.StringVar(&args.Input, "input", "", "")

	flag.StringVar(&args.Output, "o", "", "")
	flag.StringVar(&args.Output, "output", "", "")

	flag.BoolVar(&args.InPlace, "f", false, "")
	flag.BoolVar(&args.InPlace, "inplace", false, "")

	flag.BoolVar(&args.NoConfirm, "y", false, "")
	flag.BoolVar(&args.NoConfirm, "yes", false, "")

	flag.BoolVar(&args.UseZip, "zip", false, "")

	flag.Usage = func() {
		os.Stderr.WriteString(helpText)
	}

	flag.Parse()

	if args.Input == "" {
		flag.Usage()
		os.Exit(1)
	}

	return args
}
