package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jroimartin/gfm/feedmailer"
)

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}
	fm := feedmailer.NewFeedMailer()
	if err := fm.Start(flag.Arg(0)); err != nil {
		log.Fatalln(err)
	}
	log.Fatalln(<-fm.ErrChan)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gfm profile_file")
}
