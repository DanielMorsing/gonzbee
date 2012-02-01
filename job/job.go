package job

import (
	"gonzbee/config"
	"gonzbee/nntp"
	"gonzbee/nzb"
	"gonzbee/yenc"
	"os"
	"path"
	"path/filepath"
)

type Job struct {
	Name string
	Nzb  *nzb.Nzb
}

func FromFile(filepath string) (*Job, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	nzbFile, err := nzb.ParseNzb(file)
	if err != nil {
		return nil, err
	}
	j := &Job{Name: path.Base(filepath), Nzb: nzbFile}
	return j, nil
}

func (j *Job) Start(nntpConn *nntp.Conn) error {
	path := config.C.GetIncompleteDir()
	jobDir := filepath.Join(path, j.Name)
	os.Mkdir(jobDir, 0777)
	for _, file := range j.Nzb.File {
		nntpConn.SwitchGroup(file.Groups[0])
		for _, seg := range file.Segments {
			contents, err := nntpConn.GetMessage(seg.MsgId)
			if err != nil {
				continue
			}
			yenc.Decode(contents)
		}
	}
	return nil
}
