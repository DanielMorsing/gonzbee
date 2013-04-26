//Copyright 2013, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"fmt"
	"github.com/DanielMorsing/gonzbee/nntp"
	"github.com/DanielMorsing/gonzbee/yenc"
	"net"
	"net/textproto"
	"os"
	"sync"
)

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
			c, err := dialNNTP()
			if err != nil {
				if e, ok := err.(net.Error); ok {
					fmt.Fprintln(os.Stderr, "Could not connect to server:", e)
				} else if e, ok := err.(*textproto.Error); ok {
					// nothing of what we've done so far should
					fmt.Fprintln(os.Stderr, "nntp error:", e)
				} else {
					fmt.Fprintln(os.Stderr, e)
				}
				os.Exit(1)
			}
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

// Invalidate the connection if it's a permanent network error
func putConnErr(c *nntp.Conn, err error) {
	switch err.(type) {
	case yenc.DecodeError, *textproto.Error:
		putConn(c)
	default:
		putBroken(c)
	}
}

func putBroken(c *nntp.Conn) {
	c.Close()
	connMu.Lock()
	connNum--
	connMu.Unlock()
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
