package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"mizubot-go/internal/pagemonitor"
)

func main() {
	selector := flag.String("selector", "", "CSS selector to extract (e.g. #availability)")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: checkurl [-selector <css>] <url>")
		os.Exit(1)
	}
	url := flag.Arg(0)

	fmt.Printf("URL      : %s\n", url)
	fmt.Printf("Selector : %q\n", *selector)
	fmt.Println()

	result, err := pagemonitor.CheckURL(context.Background(), url, *selector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Hash     : %s\n\n", result.Hash)
	fmt.Printf("Content  :\n%s\n", result.Content)
}
