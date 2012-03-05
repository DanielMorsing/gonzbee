package main

import (
	"bytes"
	"gonzbee/nntp"
	"gonzbee/nzb"
	"gonzbee/yenc"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type job struct {
	dir string
	n   *nzb.Nzb
}

type messagejob struct {
	group string
	msgId string
	ch    chan []byte
}

func init() {
	go poolHandler()
}

var download = make(chan *messagejob)
var downloadMux = make(chan *messagejob)
var reaper = make(chan int)

func newConnection() error {
	s := config.Server.GetAddressStr()
	var err error
	var n *nntp.Conn
	if config.Server.TLS {
		n, err = nntp.DialTLS(s)
	} else {
		n, err = nntp.Dial(s)
	}
	if err != nil {
		return err
	}

	err = n.Authenticate(config.Server.Username, config.Server.Password)
	if err != nil {
		n.Close()
		return err
	}
	log.Println("spun up nntp connection")
	go func() {
		defer n.Close()
		for {
			select {
			case m := <-downloadMux:
				err = n.SwitchGroup(m.group)
				if err != nil {
					panic(err)
				}
				b, err := n.GetMessage(m.msgId)
				if err != nil {
					log.Print("Error getting Message ", m.msgId, ": ", err.Error())
				}
				m.ch <- b
			case <-(time.After(10 * time.Second)):
				reaper <- 1
				return
			}
		}
	}()
	return nil
}

func poolHandler() {
	var number int
	for {
		select {
		case msg := <-download:
			if number < 10 {
				err := newConnection()
				if err == nil {
					number++
				}
			}
			downloadMux <- msg
		case <-reaper:
			number--
		}
	}
}

func (j *job) handle() {
	wg := new(sync.WaitGroup)
	for _, f := range j.n.File {
		ch := make(chan []byte, 1024)
		wg.Add(1)
		partsLeft := len(f.Segments)
		go func() {
			var file *os.File
			var part *yenc.Part
			var err error
			defer wg.Done()
			for ; partsLeft > 0; partsLeft-- {
				s := <-ch
				if s == nil {
					continue
				}
				m := bytes.NewReader(s)
				part, err = yenc.NewPart(m)
				if err != nil {
					log.Print(err.Error())
					continue
				}
				if file == nil {
					file, err = os.Create(filepath.Join(j.dir, part.Filename))
					if err != nil {
						panic("could not create file: " + err.Error())
					}
					defer file.Close()
				}
				file.Seek(part.Begin, os.SEEK_SET)
				part.Decode(file)
			}
			if part != nil {
				log.Print("Done Decoding file " + part.Filename)
			} else {
				log.Print("Could not decode entire file")
			}
		}()
		for _, seg := range f.Segments {
			msg := &messagejob{
				msgId: seg.MsgId,
				group: f.Groups[0],
				ch:    ch,
			}
			download <- msg
		}
	}
	wg.Wait()
}

func jobStart(n *nzb.Nzb, name string) {
	incDir := config.GetIncompleteDir()
	workDir := filepath.Join(incDir, name)
	os.Mkdir(workDir, 0777)
	j := &job{
		dir: workDir,
		n:   n,
	}
	j.handle()
}
