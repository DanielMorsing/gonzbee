package main

import (
	"errors"
	"flag"
	"fmt"
	"gonzbee/config"
	"gonzbee/job"
	"gonzbee/nzb"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
)

func panicOn(err interface{}) {
	if err != nil {
		panic(err)
	}
}

var (
	profile = flag.String("profile", "", "Where to save cpuprofile data")
	rm      = flag.Bool("rm", false, "Remove the nzb file after downloading")
)

func main() {
	defer func() {
		if e := recover(); e != nil {
			err := e.(error)
			fmt.Fprintln(os.Stdout, err.Error())
			os.Exit(1)
		}
	}()
	flag.Parse()
	runtime.GOMAXPROCS(4)
	if *profile != "" {
		pfile, err := os.Create(*profile)
		if err != nil {
			panic(errors.New("Could not open profile file"))
		}
		err = pprof.StartCPUProfile(pfile)
		if err != nil {
			panic(err)
		}
		defer pfile.Close()
		defer pprof.StopCPUProfile()
	}

	fmt.Println(config.C)
	nzbPath := flag.Arg(0)
	file, err := os.Open(nzbPath)
	panicOn(err)

	n, err := nzb.Parse(file)
	panicOn(err)
	file.Close()
	if *rm {
		err = os.Remove(nzbPath)
		panicOn(err)
	}

	job.Start(n, filepath.Base(nzbPath))
}
