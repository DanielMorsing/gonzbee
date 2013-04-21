//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/DanielMorsing/gonzbee/nntp"
	"github.com/DanielMorsing/gonzbee/nzb"
	"github.com/DanielMorsing/gonzbee/yenc"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
)

var (
	rm      = flag.Bool("rm", false, "Remove the nzb file after downloading")
	saveDir = flag.String("d", "", "Save to this directory")
	par     = flag.Int("par", 0, "How many par2 parts to download")
)

var extStrip = regexp.MustCompile(`\.nzb$`)

var existErr = errors.New("file exists")

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "No NZB files given")
		os.Exit(1)
	}

	if *par != 0 {
		if flag.NArg() != 1 {
			*par = 0
			fmt.Fprintln(os.Stderr, "par option not supported with multiple downloads. Not downloading pars.")
		}
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

		filterPars(nzb, *par)

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

var parregexp = regexp.MustCompile(`(?i)\+(\d+).par2$`)

func filterPars(nfile *nzb.Nzb, npars int) bool {
	files := make([]*nzb.File, 0, len(nfile.File))
	parfiles := make([]*parfile, 0, len(nfile.File))
	if npars < 0 {
		return true
	}

	var sum int
	for _, f := range nfile.File {
		s := parregexp.FindStringSubmatch(f.Subject.Filename())
		if s == nil {
			files = append(files, f)
		} else {
			p, err := strconv.ParseInt(s[1], 10, 0)
			if err != nil {
				// be safe and just add this to the download set
				files = append(files, f)
				continue
			}
			sum += int(p)
			parfiles = append(parfiles, &parfile{int(p), f})
		}
	}

	// Selecting the set of par files is fairly tricky.
	// it can be boiled down to an inverted knapsack problem.
	//
	target := sum - npars

	if target < 1 {
		// want more blocks than available, or the exact amount.
		return target == 0
	}

	// the plus one here is not an error. it's to make space for the empty case
	m := make([][]int, len(parfiles)+1)
	keep := make([][]bool, len(parfiles)+1)
	for i := range m {
		// also not an error, it's to make space for both 0 and target inclusively
		m[i] = make([]int, target+1)
		keep[i] = make([]bool, target+1)
	}
	// this is the standard dynamic programming solution for the knapsack problem.
	for i := 1; i <= len(parfiles); i++ {
		w := parfiles[i-1].n
		for j := 0; j <= target; j++ {
			if j >= w {
				oldValue := m[i-1][j]
				potentialVal := m[i-1][j-w] + w
				if oldValue < potentialVal {
					m[i][j] = potentialVal
					keep[i][j] = true
				} else {
					m[i][j] = oldValue
				}
			} else {
				m[i][j] = m[i-1][j]
			}
		}
	}

	// walk through the keep matrix
	excludeSet := make([]*parfile, 0, len(parfiles))
	i, j := len(parfiles), target
	for i > 0 {
		if keep[i][j] {
			j -= parfiles[i-1].n
			excludeSet = append(excludeSet, parfiles[i-1])
		}
		i -= 1
	}

	// this is a bit of a hack. Since i know that excludeset
	// was added to in reverse order, i can do a linear scan here
	// going backwards in excludeset and forwards in parfiles
	i, j = 0, len(excludeSet)-1
	for j >= 0 {
		if parfiles[i] == excludeSet[j] {
			j--
		} else {
			files = append(files, parfiles[i].file)
		}
		i++
	}
	// add the remaining parfiles to the set.
	for ; i < len(parfiles); i++ {
		files = append(files, parfiles[i].file)
	}
	nfile.File = files
	return true
}

type parfile struct {
	n    int
	file *nzb.File
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
		c := getConn()
		go decodeMsg(c, file, nzbfile.Groups, f.MsgId)
	}
	return nil
}

func decodeMsg(c *nntp.Conn, f *file, groups []string, MsgId string) {
	var err error
	defer func() { putConnErr(c, err) }()
	defer f.Done()
	err = findGroup(c, groups)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nntp error:", err)
		return
	}
	rc, err := c.GetMessageReader(MsgId)
	defer rc.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nntp error:", err)
		return
	}
	yread, err := yenc.NewPart(rc)
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

type fileWriter struct {
	f      *os.File
	offset int64
}

func (f *fileWriter) Write(b []byte) (int, error) {
	n, err := f.f.WriteAt(b, f.offset)
	f.offset += int64(n)
	return n, err
}
