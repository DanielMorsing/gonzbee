package main

import (
	"flag"
	"fmt"
	"gonzbee/config"
	"gonzbee/job"
	"gonzbee/nzb"
	"os"
	"path/filepath"
)

func panicOn(err interface{}) {
	if err != nil {
		panic(err)
	}
}

func main() {
	defer func() {
		if e := recover(); e != nil {
			err := e.(error)
			fmt.Fprintln(os.Stdout, err.Error())
			os.Exit(1)
		}
	}()
	flag.Parse()

	fmt.Println(config.C)
	nzbPath := flag.Arg(0)
	file, err := os.Open(nzbPath)
	panicOn(err)

	n, err := nzb.Parse(file)
	panicOn(err)
	file.Close()

	job.Start(n, filepath.Base(nzbPath))
}
