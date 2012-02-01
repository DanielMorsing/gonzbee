package nntp

import (
	"net/textproto"
)

type Conn struct {
	*textproto.Conn
}

func Dial(address string) (*Conn, error) {
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
	return n, nil
}

func (n *Conn) Authenticate(user, pass string) error {
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

func (n *Conn) SwitchGroup(group string) error {
	id, err := n.Cmd("GROUP %s", group)
	if err != nil {
		return err
	}
	n.StartResponse(id)
	_, _, err = n.ReadCodeLine(211)
	n.EndResponse(id)
	return err
}

func (n *Conn) GetMessage(msgId string) ([]byte, error) {
	id, err := n.Cmd("BODY <%s>", msgId)
	if err != nil {
		return nil, err
	}
	n.StartResponse(id)
	defer n.EndResponse(id)
	_, _, err = n.ReadCodeLine(222)
	if err != nil {
		return nil, err
	}
	return n.ReadDotBytes()
}
