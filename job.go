package main

import (
	"github.com/DanielMorsing/gonzbee/nntp"
	"github.com/DanielMorsing/gonzbee/nzb"
	"github.com/DanielMorsing/gonzbee/yenc"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type job struct {
	dir string
	n   *nzb.Nzb
}

var downloaderRq = make(chan chan *nntp.Conn)
var downloadReaper = make(chan *nntp.Conn, 1024)

func init() {
	go connectionHandler()
}

func spinUp() *nntp.Conn {
	str := config.GetAddressStr()
	var conn *nntp.Conn
	var err error
	if config.TLS {
		conn, err = nntp.DialTLS(str)
	} else {
		conn, err = nntp.Dial(str)
	}
	if err != nil {
		return nil
	}
	err = conn.Authenticate(config.Username, config.Password)
	if err != nil {
		return nil
	}
	return conn
}

func connectionHandler() {
	var number int
	for {
		ch := <-downloaderRq
		var conn *nntp.Conn
		if number < 10 {
			conn = spinUp()
			if conn == nil {
				continue
			}
			log.Print("Spun up connection #", number+1)
			number++
			ch <- conn
			continue
		}
		conn = <-downloadReaper
		ch <- conn
	}
}

type offsetWriter struct {
	offset int64
	io.WriterAt
}

func (ow *offsetWriter) Write(b []byte) (n int, err error) {
	n, err = ow.WriteAt(b, ow.offset)
	ow.offset += int64(n)
	return
}

func (j *job) handle() {
	var jobDone sync.WaitGroup
	jobDone.Add(len(j.n.File))
	for _, f := range j.n.File {
		var fileinit sync.Once
		var file *os.File
		var fileClose sync.WaitGroup
		fileClose.Add(len(f.Segments))
		for _, s := range f.Segments {
			ch := make(chan *nntp.Conn)
			downloaderRq <- ch
			conn := <-ch
			go func(seg nzb.Segment, f nzb.File) {
				defer fileClose.Done()
				defer func() {
					downloadReaper <- conn
				}()

				err := conn.SwitchGroup(f.Groups[0])
				if err != nil {
					return
				}

				reader, err := conn.GetMessageReader(seg.MsgId)
				if err != nil {
					return
				}
				defer reader.Close()

				part, err := yenc.NewPart(reader)
				if err != nil {
					return
				}

				fileinit.Do(func() {
					file, err = os.Create(filepath.Join(j.dir, part.Filename))
					if err != nil {
						os.Exit(1)
					}
					go func() {
						fileClose.Wait()
						file.Close()
						jobDone.Done()
						log.Printf("Done downloading file \"%s\"\n", part.Filename)
					}()
				})

				ow := &offsetWriter{
					offset:   part.Begin,
					WriterAt: file,
				}
				io.Copy(ow, part)
			}(s, f)
		}
	}
	jobDone.Wait()
}

func jobStart(n *nzb.Nzb, name string, dir string) {
	workDir := filepath.Join(dir, name)
	os.Mkdir(workDir, 0777)
	j := &job{
		dir: workDir,
		n:   n,
	}
	j.handle()
}
