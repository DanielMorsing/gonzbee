//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/DanielMorsing/gonzbee/nntp"
	"github.com/DanielMorsing/gonzbee/nzb"
	"github.com/DanielMorsing/gonzbee/par2"
	"github.com/DanielMorsing/gonzbee/yenc"
)

var (
	rm       = flag.Bool("rm", false, "Remove the nzb file after downloading")
	saveDir  = flag.String("d", "", "Save to this directory")
	par      = flag.Bool("par", false, "override par2 download logic and download all files")
	profAddr = flag.String("prof", "", "address to open profiling server on")
)

var extStrip = regexp.MustCompile(`\.nzb$`)

var existErr = errors.New("file exists")

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "No NZB files given")
		os.Exit(1)
	}

	if *profAddr != "" {
		laddr, err := net.Listen("tcp", *profAddr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		go func() {
			log.Fatalln(http.Serve(laddr, nil))
		}()
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

// download all the files contained in an nzb,
func downloadNzb(nzbFile *nzb.Nzb, dir string) error {
	if *saveDir != "" {
		dir = *saveDir
	}
	err := os.Mkdir(dir, os.ModePerm)
	// if the directory already exist, assume that it's an old download that was canceled
	// and restarted.
	if err != nil && !os.IsExist(err) {
		return err
	}
	var parfiles map[*nzb.File][]*parfile
	if !*par {
		parfiles = filterPars(nzbFile)
		// first download the parfiles
		for file, _ := range parfiles {
			err = downloadFile(dir, file)
			if err == existErr {
				continue
			} else if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}

	// download the rest of the files.
	for _, file := range nzbFile.File {
		err = downloadFile(dir, file)
		if err == existErr {
			continue
		} else if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	if *par {
		return nil
	}

	// create a list of files downloaded
	var paths []string
	for _, file := range nzbFile.File {
		filename := file.Subject.Filename()
		path := filepath.Join(dir, filename)
		paths = append(paths, path)
	}
	filewg.Wait()
	for fp, set := range parfiles {
		var n int
		paths, n, err = verifyPar(fp, dir, paths)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		files := selectPars(set, n)

		for _, file := range files {
			err = downloadFile(dir, file)
			if err == existErr {
				continue
			} else if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}
	return nil
}

func verifyPar(fp *nzb.File, dir string, paths []string) ([]string, int, error) {
	filename := fp.Subject.Filename()
	path := filepath.Join(dir, filename)
	f, err := os.Open(path)
	if err != nil {
		return paths, 0, err
	}
	defer f.Close()
	fset := par2.NewFileset(f)
	if !fset.CanVerify() {
		return paths, 0, nil
	}
	pathSet := make(map[string]bool)
	for _, s := range paths {
		pathSet[s] = true
	}
	matches, blockNeeded := fset.Verify(paths)
	for _, fm := range matches {
		if pathSet[fm.Path] {
			delete(pathSet, fm.Path)
		}
	}
	retPaths := make([]string, 0, len(pathSet))
	for s := range pathSet {
		retPaths = append(retPaths, s)
	}
	return retPaths, blockNeeded, nil
}

// download a single file contained in an nzb.
func downloadFile(dir string, nzbfile *nzb.File) error {
	file, err := newFile(dir, nzbfile)
	if err != nil {
		return err
	}
	for _, f := range nzbfile.Segments {
		c, err := getConn()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		go decodeMsg(c, file, nzbfile.Groups, f.MsgId)
	}
	return nil
}

// decodes an nntp message and writes it to a section of the file.
func decodeMsg(c *nntp.Conn, f *file, groups []string, MsgId string) {
	var err error
	defer f.Done()
	err = findGroup(c, groups)
	if err != nil {
		putBroken(c)
		fmt.Fprintln(os.Stderr, "nntp error:", err)
		return
	}
	rc, err := c.GetMessage(MsgId)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nntp error getting", MsgId, ":", err)
		if _, ok := err.(*textproto.Error); ok {
			putConn(c)
		} else {
			putBroken(c)
		}
		return
	}
	putConn(c)

	yread, err := yenc.NewPart(bytes.NewReader(rc))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	wr := f.WriterAt(yread.Begin)
	_, err = io.Copy(wr, yread)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func findGroup(c *nntp.Conn, groups []string) error {
	var err error
	for _, g := range groups {
		err = c.SwitchGroup(g)
		if err == nil {
			return nil
		}
	}
	return err
}

// waitgroup that keeps track if there are any files being downloaded.
var filewg sync.WaitGroup

type file struct {
	name      string
	path      string
	file      *os.File
	partsLeft int
	mu        sync.Mutex
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
	}
	filewg.Add(1)
	return ret, nil
}

func (f *file) WriterAt(offset int64) io.Writer {
	return &fileWriter{
		f:      f.file,
		offset: offset,
	}
}

func (f *file) Done() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.partsLeft--
	if f.partsLeft != 0 {
		return
	}
	fmt.Printf("Done downloading file %q\n", f.name)
	os.Rename(f.file.Name(), f.path)
	f.file.Close()
	filewg.Done()

}

// filewriter allows for multiple goroutines to write concurrently to
// non-overlapping sections of a file
type fileWriter struct {
	f      *os.File
	offset int64
}

func (f *fileWriter) Write(b []byte) (int, error) {
	n, err := f.f.WriteAt(b, f.offset)
	f.offset += int64(n)
	return n, err
}
