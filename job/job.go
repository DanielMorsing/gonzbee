package job

import (
	"gonzbee/config"
	"gonzbee/nntp"
	"gonzbee/nzb"
	"gonzbee/yenc"
	"io"
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
	ch    chan io.ReadCloser
}

func init() {
	go poolHandler()
}

var download = make(chan *messagejob)
var downloadMux = make(chan *messagejob)
var reaper = make(chan int)

func newConnection() error {
	s := config.C.Server.GetAddressStr()
	var err error
	var n *nntp.Conn
	if config.C.Server.TLS {
		n, err = nntp.DialTLS(s)
	} else {
		n, err = nntp.Dial(s)
	}
	if err != nil {
		return err
	}

	err = n.Authenticate(config.C.Server.Username, config.C.Server.Password)
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
				b, err := n.GetMessageReader(m.msgId)
				if err != nil {
					log.Print("Error getting Message ", m.msgId, ": ", err.Error())
				}
				m.ch <- b
			case <-(after(10 * time.Second)):
				reaper <- 1
				return
			}
		}
	}()
	return nil
}

func after(d time.Duration) <-chan time.Time {
	t := time.NewTimer(d)
	return t.C
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
		ch := make(chan io.ReadCloser)
		wg.Add(1)
		partsLeft := len(f.Segments)
		go func() {
			var file *os.File
			var part *yenc.Part
			var err error
			defer wg.Done()
			for ; partsLeft > 0; partsLeft-- {
				m := <-ch
				if m == nil {
					continue
				}
				part, err = yenc.NewPart(m)
				if err != nil {
					log.Print(err.Error())
					m.Close()
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
				m.Close()
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

func Start(n *nzb.Nzb, name string) {
	incDir := config.C.GetIncompleteDir()
	workDir := filepath.Join(incDir, name)
	os.Mkdir(workDir, 0777)
	j := &job{
		dir: workDir,
		n:   n,
	}
	j.handle()
}
