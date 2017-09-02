//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

//Package nntp provides some common operations on nntp server,
//mostly for binary downloading
package nntp

import (
	"crypto/tls"
	"net/textproto"
)

//Conn represents a NNTP connection
type Conn struct {
	*textproto.Conn
}

//Dial will establish a connection to a NNTP server.
//It returns the connection and an error, if any
func Dial(address, user, pass string) (*Conn, error) {
	n := new(Conn)
	var err error
	n.Conn, err = textproto.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	_, _, err = n.ReadCodeLine(20)
	if err != nil {
		n.Close()
		return nil, err
	}
	err = n.authenticate(user, pass)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func DialTLS(address, user, pass string) (*Conn, error) {
	n := new(Conn)
	tlsConn, err := tls.Dial("tcp", address, nil)
	if err != nil {
		return nil, err
	}
	n.Conn = textproto.NewConn(tlsConn)
	_, _, err = n.ReadCodeLine(20)
	if err != nil {
		n.Close()
		return nil, err
	}
	err = n.authenticate(user, pass)
	if err != nil {
		return nil, err
	}
	return n, nil
}

//Authenticate will authenticate with the NNTP server, using the supplied
//username and password. It returns an error, if any
func (n *Conn) authenticate(user, pass string) error {
	id, err := n.Cmd("AUTHINFO USER %s", user)
	if err != nil {
		return err
	}
	n.StartResponse(id)
	code, _, err := n.ReadCodeLine(381)
	n.EndResponse(id)
	switch code {
	case 481, 482, 502:
		//failed, out of sequence or command not available
		return err
	case 281:
		//accepted without password
		return nil
	case 381:
		//need password
		break
	default:
		return err
	}
	id, err = n.Cmd("AUTHINFO PASS %s", pass)
	if err != nil {
		return err
	}
	n.StartResponse(id)
	code, _, err = n.ReadCodeLine(281)
	n.EndResponse(id)
	return err
}

//GetMessage will retrieve a message from the server, using the supplied
//msgId. It returns the contents of the message and an error, if any
func (n *Conn) GetMessage(msgId string) ([]byte, error) {
	id, err := n.Cmd("BODY <%s>", msgId)
	// A bit of synchronization weirdness. If one of the cmd sends in a pipeline fail
	// while another is waiting for a response, we want to signal that our response has
	// been read anyway. This gives waiters in the pipeline the opportunity to wake up
	// realize the connection is closed
	n.StartResponse(id)
	defer n.EndResponse(id)
	if err != nil {
		return nil, err
	}
	_, _, err = n.ReadCodeLine(222)
	if err != nil {
		return nil, err
	}
	return n.ReadDotBytes()
}
