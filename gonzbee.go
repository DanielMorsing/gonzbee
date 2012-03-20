package main

import (
	"errors"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/DanielMorsing/gonzbee/nzb"
	"os"
	"io/ioutil"
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
	profile = flag.String("profile", "", "Where to save profile data")
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
	if *profile != "" {
		cpuprof := *profile + ".pprof"
		pfile, err := os.Create(cpuprof)
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

	if *profile != "" {
		memprof := *profile + ".memprof"
		pfile, err := os.Create(memprof)
		if err != nil {
			panic(errors.New("Could not create profile file"))
		}
		defer pfile.Close()
		err = pprof.WriteHeapProfile(pfile)
		if err != nil {
			panic(err)
		}

		var memstat runtime.MemStats
		runtime.ReadMemStats(&memstat)
		b, err := json.MarshalIndent(memstat, "", "\t")
		if err != nil {
			panic(err)
		}
		err = ioutil.WriteFile(*profile + ".memstats", b, 0644)
		if err != nil {
			panic(err)
		}
	}
}
