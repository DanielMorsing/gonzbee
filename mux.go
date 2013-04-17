//Copyright 2013, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"fmt"
	"github.com/DanielMorsing/gonzbee/nntp"
	"io"
	"net"
	"net/textproto"
	"os"
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
		if e, ok := err.(net.Error); ok && !e.Temporary() {
			c.Close()
			connMu.Lock()
			connNum--
			connMu.Unlock()
			return nil, err
		}
	}
	putConn(c)
	return nil, err

found:
	r, err := c.GetMessageReader(msgId)
	if err != nil {
		if e, ok := err.(net.Error); ok && !e.Temporary() {
			c.Close()
			connMu.Lock()
			connNum--
			connMu.Unlock()
			return nil, err
		}
		putConn(c)
		return nil, err
	}

	reader := msgReader{r, c, false}
	return &reader, nil
}

type msgReader struct {
	io.ReadCloser
	*nntp.Conn
	invalid bool
}

func (m *msgReader) Read(b []byte) (n int, err error) {
	n, err = m.ReadCloser.Read(b)
	if e, ok := err.(net.Error); ok && !e.Temporary() {
		m.invalid = true
	}
	return
}

func (m *msgReader) Close() error {
	err := m.ReadCloser.Close()
	if !m.invalid {
		putConn(m.Conn)
	} else {
		m.Conn.Close()
		connMu.Lock()
		connNum--
		connMu.Unlock()
	}
	return err
}

var (
	connMu  sync.Mutex
	connNum int
	connCh  = make(chan *nntp.Conn, 10)
	errCh   = make(chan error, 10)
)

func getConn() (*nntp.Conn) {
	connMu.Lock()

	if connNum < 10 {
		connNum++
		go func() {
			c, err := dialNNTP()
			if err != nil {
				errCh <- err
				return
			}
			connCh <- c
		}()
	}
	connMu.Unlock()
	select {
	case c := <-connCh:
		return c
	case err := <-errCh:
		// Most errors here will be permanent errors that we cannot help
		// just bomb out
		if e, ok := err.(net.Error); ok {
			fmt.Fprintln(os.Stderr, "Could not connect to server:", e)
		} else if e, ok := err.(*textproto.Error); ok {
			// nothing of what we've done so far should
			fmt.Fprintln(os.Stderr, "nntp error:", e)
		}
		os.Exit(1)
	}
	panic("unreachable")
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
	if err != nil {
		return nil, err
	}
	return c, nil
}
