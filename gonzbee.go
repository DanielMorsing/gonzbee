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
	"net/textproto"
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

var nntpChan = make(chan *nntp.Conn, 10)

func getNNTP() (*nntp.Conn, error) {
	dialstr := config.GetAddressStr()
	var err error
	var c *nntp.Conn

	c = <-nntpChan
	if c != nil {
		return c, nil
	}

	if config.TLS {
		c, err = nntp.DialTLS(dialstr)
	} else {
		c, err = nntp.Dial(dialstr)
	}
	if err != nil {
		return nil, err
	}
	err = c.Authenticate(config.Username, config.Password)
	return c, nil
}

func putNNTP(c *nntp.Conn) {
	nntpChan <- c
}

func init() {
	for i := 0; i < 10; i++ {
		nntpChan <- nil
	}
}

func main() {
	flag.Parse()
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
		file.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			continue
		}

		err = downloadNzb(nzb, extStrip.ReplaceAllString(path, ""))
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
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
	var wg sync.WaitGroup
	for _, file := range nzbFile.File {
		conn, err := getNNTP()
		if err != nil {
			return err
		}
		wg.Add(1)
		go func(c *nntp.Conn, file *nzb.File) {

			defer putNNTP(c)
			defer wg.Done()
			var i int
			var err error
			// nntp server might not have a given group, try them all
			for i = 0; i < len(file.Groups); i++ {
				err = conn.SwitchGroup(file.Groups[i])
				if err == nil {
					break
				}
			}
			if i == len(file.Groups) {
				fmt.Println(os.Stderr, err)
				return
			}

			err = downloadFile(c, dir, file)
			if err == existErr {
				return
			} else if err != nil {
				fmt.Println(os.Stderr, err)
			}
		}(conn, file)
	}
	wg.Wait()
	return nil
}

func downloadFile(conn *nntp.Conn, dir string, nzbfile *nzb.File) error {
	var file *os.File
	var fname string
	var err error

	fname = nzbfile.Subject.Filename()
	if fname == "" {
		return errors.New("bad subject")
	}
	fname = filepath.Join(dir, fname)
	tmpname := fname + ".gonztemp"

	file, err = os.Create(tmpname)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, seg := range nzbfile.Segments {
		err = getYenc(conn, file, seg.MsgId)
		if err != nil {
			return err
		}
	}
	return os.Rename(file.Name(), fname)
}

func getYenc(c *nntp.Conn, f *os.File, msgid string) error {
	s, err := c.GetMessageReader(msgid)
	if e, ok := err.(*textproto.Error); ok && e.Code == 430 {
		fmt.Fprintf(os.Stderr, "Missing segment %q\n", msgid)
		return nil
	} else if err != nil {
		return err
	}
	defer s.Close()
	y, err := yenc.NewPart(s)
	if err != nil {
		return err
	}
	return writeYenc(f, y)
}

func writeYenc(f *os.File, y *yenc.Part) error {
	_, err := f.Seek(y.Begin, 0)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, y)
	return err
}
