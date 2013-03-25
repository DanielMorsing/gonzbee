//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/DanielMorsing/gonzbee/nzb"
	"github.com/DanielMorsing/gonzbee/yenc"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var (
	rm      = flag.Bool("rm", false, "Remove the nzb file after downloading")
	saveDir = flag.String("d", "", "Save to this directory")
)

var extStrip = regexp.MustCompile(`\.nzb$`)

var existErr = errors.New("file exists")

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "No NZB files given")
		os.Exit(1)
	}

	for _, path := range flag.Args() {
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		nzb, err := nzb.Parse(file)
		file.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		err = downloadNzb(nzb, extStrip.ReplaceAllString(path, ""))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
	}
}

func downloadNzb(nzbFile *nzb.Nzb, dir string) error {
	if *saveDir != "" {
		dir = *saveDir
	}
	err := os.Mkdir(dir, os.ModePerm)
	if err != nil && !os.IsExist(err) {
		return err
	}
	for _, file := range nzbFile.File {
		err = downloadFile(dir, file)
		if err == existErr {
			continue
		} else if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	return nil
}

func downloadFile(dir string, nzbfile *nzb.File) error {
	var file *os.File
	var fname string
	var err error

	fname = nzbfile.Subject.Filename()
	if fname == "" {
		return errors.New("bad subject")
	}
	fname = filepath.Join(dir, fname)
	if _, err := os.Stat(fname); err == nil {
		return existErr
	}
	tmpname := fname + ".gonztemp"

	file, err = os.Create(tmpname)
	if err != nil {
		return err
	}
	defer file.Close()
	retCh := make(chan *getResult)
	go func() {
		for i := range nzbfile.Segments {
			getCh <- &getRequest{
				retCh,
				nzbfile.Segments[i].MsgId,
				nzbfile.Groups,
			}
		}
	}()
	for i := 0; i < len(nzbfile.Segments); i++ {
		ret := <-retCh
		if ret.err != nil {
			fmt.Fprintln(os.Stderr, ret.err)
			continue
		}
		err := writeYenc(file, ret.ret)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	return os.Rename(file.Name(), fname)
}

func writeYenc(f *os.File, b []byte) error {
	rd := bytes.NewReader(b)
	y, err := yenc.NewPart(rd)
	if err != nil {
		return err
	}

	_, err = f.Seek(y.Begin, 0)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, y)
	return err
}
