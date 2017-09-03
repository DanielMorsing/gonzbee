//Copyright 2013, Daniel Morsing
//For licensing information, See the LICENSE file

// This file contains a muxer that will limit the amount of connections
// that are concurrently running.

package main

import (
	"net"
	"sync"

	"github.com/DanielMorsing/gonzbee/nntp"
)

var (
	connMu  sync.Mutex
	connNum int
	connCh  = make(chan *nntp.Conn, maxNumConn*pipelineDepth)
)

const (
	maxNumConn    = 20
	pipelineDepth = 10
)

func getConn() (*nntp.Conn, error) {
	// check if there's a free conn we can get
	select {
	case c := <-connCh:
		return c, nil
	default:
	}
	connMu.Lock()
	if connNum == maxNumConn {
		connMu.Unlock()
		// wait for idle conn
		select {
		case c := <-connCh:
			return c, nil
		}
	}
	connNum++
	connMu.Unlock()
	type connerr struct {
		c   *nntp.Conn
		err error
	}
	ch := make(chan connerr)
	cancelch := make(chan struct{})

	// dial with this connection.
	// if we manage to get a connection from
	// a client  done with theirs, we will use that one
	// and put the idle conn
	go func() {
		c, err := dialNNTP()
		select {
		case <-cancelch:
			if err == nil {
				for i := 0; i < pipelineDepth; i++ {
					putConn(c)
				}
				return
			}
			// ignore error
			connMu.Lock()
			connNum--
			connMu.Unlock()
		case ch <- connerr{c, err}:
			for i := 0; i < pipelineDepth-1; i++ {
				putConn(c)
			}
		}
	}()
	select {
	case ce := <-ch:
		return ce.c, ce.err
	case c := <-connCh:
		close(cancelch)
		return c, nil
	}
}

func putConn(c *nntp.Conn) {
	connCh <- c
}

func putBroken(c *nntp.Conn) {
	err := c.Close()
	if err == nntp.ErrAlreadyClosed {
		return
	}
	connMu.Lock()
	connNum--
	connMu.Unlock()
}

func dialNNTP() (*nntp.Conn, error) {
	dialstr := config.GetAddressStr()
	var err error
	var c *nntp.Conn

	for {
		if config.TLS {
			c, err = nntp.DialTLS(dialstr, config.Username, config.Password)
		} else {
			c, err = nntp.Dial(dialstr, config.Username, config.Password)
		}
		if err != nil {
			// if it's a timeout, ignore and try again
			e, ok := err.(net.Error)
			if ok && e.Temporary() {
				continue
			}
			return nil, err
		}
		break
	}
	return c, nil
}
