//Copyright 2013, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"github.com/DanielMorsing/gonzbee/nntp"
)

var getCh = make(chan *getRequest)

type getRequest struct {
	ret    chan *getResult
	msgid  string
	groups []string
}

type getResult struct {
	ret []byte
	err error
}

func init() {
	go server()
}

func server() {
	var bufCh = make(chan *nntp.Conn, 10)
	for i := 0; i < cap(bufCh); i++ {
		bufCh <- nil
	}

	for {
		var err error
		rq := <-getCh
		c := <-bufCh
		if c == nil {
			c, err = dialNNTP()
			if err != nil {
				rq.ret <- &getResult{nil, err}
				continue
			}
		}
		go func(c *nntp.Conn, rq *getRequest) {
			var err error
			for _, s := range rq.groups {
				err = c.SwitchGroup(s)
				if err == nil {
					goto Found
				}
			}
			rq.ret <- &getResult{nil, err}
			return

		Found:
			b, err := c.GetMessage(rq.msgid)
			rq.ret <- &getResult{b, err}
			bufCh <- c
		}(c, rq)
	}
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
