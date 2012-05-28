//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/DanielMorsing/gonzbee/nzb"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
)

var (
	profile   = flag.String("profile", "", "Where to save profile data")
	rm        = flag.Bool("rm", false, "Remove the nzb file after downloading")
	saveDir   = flag.String("d", "", "Save to this directory")
	aggregate = flag.String("a", "", "Save all files in all NZBs in this directory")
)

var extStrip = regexp.MustCompile(`\.nzb$`)

func main() {
	flag.Parse()
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
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "No NZB files given")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			continue
		}

		nzb, err := nzb.Parse(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			continue
		}

		var downloadDir string
		if *aggregate != "" {
			downloadDir = *aggregate
		} else {
			downloadDir = extStrip.ReplaceAllString(filepath.Base(path), "")
		}
		err = jobStart(nzb, downloadDir, *saveDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not download job: %s\n", err.Error())
			continue
		}
		if *rm {
			err = os.Remove(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
			}
		}
	}

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
		err = ioutil.WriteFile(*profile+".memstats", b, 0644)
		if err != nil {
			panic(err)
		}
	}
}
