//Copyright 2013, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"github.com/DanielMorsing/gonzbee/nntp"
	"io"
	"sync"
)

func getMessage(group []string, msgId string) (io.ReadCloser, error) {
	c := getConn()
	var err error
	for _, g := range group {
		err = c.SwitchGroup(g)
		if err == nil {
			goto found
		}
	}
	return nil, err

found:
	r, err := c.GetMessageReader(msgId)
	if err != nil {
		return nil, err
	}

	reader := msgReader{r, c, nil}
	return &reader, nil
}

type msgReader struct {
	io.ReadCloser
	*nntp.Conn
	err error
}

func (m *msgReader) Close() error {
	err := m.ReadCloser.Close()
	putConn(m.Conn)
	return err
}

var (
	connMu  sync.Mutex
	connNum int
	connCh  = make(chan *nntp.Conn, 10)
)

func getConn() *nntp.Conn {
	connMu.Lock()

	if connNum < 10 {
		connNum++
		go func() {
			c, _ := dialNNTP()
			connCh <- c
		}()
	}
	connMu.Unlock()

	c := <-connCh
	return c
}

func putConn(c *nntp.Conn) {
	connCh <- c
}

func dialNNTP() (*nntp.Conn, error) {
	dialstr := config.GetAddressStr()
	var err error
	var c *nntp.Conn

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
