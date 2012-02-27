package main

import (
	"errors"
	"flag"
	"fmt"
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
	cpuprofile = flag.String("cpuprofile", "", "Where to save cpuprofile data")
	memprofile = flag.String("memprofile", "", "Where to save memory profile data")
	rm         = flag.Bool("rm", false, "Remove the nzb file after downloading")
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
	if *cpuprofile != "" {
		pfile, err := os.Create(*cpuprofile)
		if err != nil {
			panic(errors.New("Could not create profile file"))
		}
		defer pfile.Close()
		err = pprof.StartCPUProfile(pfile)
		if err != nil {
			panic(err)
		}
		defer pprof.StopCPUProfile()
	}

	fmt.Println(config)
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

	jobStart(n, filepath.Base(nzbPath))

	if *memprofile != "" {
		pfile, err := os.Create(*memprofile)
		if err != nil {
			panic(errors.New("Could not create profile file"))
		}
		defer pfile.Close()
		err = pprof.WriteHeapProfile(pfile)
		if err != nil {
			panic(err)
		}
	}
}
