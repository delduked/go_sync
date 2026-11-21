package main

import (
	"flag"
	"fmt"
	"github.com/TypeTerrors/go_sync/pkg"
)

func completionCommand(command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	name := flags.String("file", "", "Local file to mark or verify")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *name == "" || flags.NArg() != 0 {
		return fmt.Errorf("%s requires --file PATH", command)
	}
	if command == "complete" {
		marker, err := pkg.MarkComplete(*name)
		if err != nil {
			return err
		}
		fmt.Println(marker)
		return nil
	}
	if err := pkg.VerifyComplete(*name); err != nil {
		return err
	}
	fmt.Println("verified")
	return nil
}
