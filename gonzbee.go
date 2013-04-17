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
	"sync"
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

		if *rm {
			err = os.Remove(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}
	filewg.Wait()
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
	file, err := newFile(dir, nzbfile)
	if err != nil {
		return err
	}
	for _, f := range nzbfile.Segments {
		rc, err := getMessage(nzbfile.Groups, f.MsgId)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error getting", f.MsgId, ":", err)
			file.rq <- nil
			continue
		}
		go decodeMsg(rc, file)
	}
	return nil
}

func decodeMsg(rc io.ReadCloser, f *file) {
	var b bytes.Buffer
	yread, err := yenc.NewPart(rc)
	if err != nil {
		f.rq <- nil
	}
	b.Grow(int(yread.Size))
	io.Copy(&b, yread)
	rc.Close()
	f.rq <- &yencResult{b.Bytes(), yread.Begin}
}

func newFile(dirname string, nzbfile *nzb.File) (*file, error) {
	filename := nzbfile.Subject.Filename()
	if filename == "" {
		return nil, errors.New("bad subject")
	}

	path := filepath.Join(dirname, filename)
	if _, err := os.Stat(path); err == nil {
		return nil, existErr
	}

	temppath := path + ".gonztemp"
	f, err := os.Create(temppath)
	if err != nil {
		return nil, err
	}

	ret := &file{
		name:      filename,
		path:      path,
		partsLeft: len(nzbfile.Segments),
		file:      f,
		rq:        make(chan *yencResult, len(nzbfile.Segments)),
	}
	filewg.Add(1)
	go ret.serve()

	return ret, nil
}

var filewg sync.WaitGroup

func (f *file) serve() {
	for ; f.partsLeft > 0; f.partsLeft-- {
		y := <-f.rq
		if y == nil {
			continue
		}
		f.file.Seek(y.offset, os.SEEK_SET)
		f.file.Write(y.b)
	}
	fmt.Printf("Done downloading file %q\n", f.name)
	os.Rename(f.file.Name(), f.path)
	f.file.Close()
	filewg.Done()
}

type file struct {
	name      string
	path      string
	file      *os.File
	partsLeft int
	rq        chan *yencResult
}

type yencResult struct {
	b      []byte
	offset int64
}
