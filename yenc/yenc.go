//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

//Package yenc gives a decoding interface for yEnc encoded binaries.
package yenc

import (
	"bufio"
	"bytes"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

//Part holds the information contained in a parsed yEnc header.
//It also implements the read interface for easy decoding.
//Normally, you will need to read the Filename to find out which file to open
//and Begin to know where to seek before writing.
type Part struct {
	Filename  string
	Begin     int64
	Size      int64
	NumParts  int
	Number    int
	end       int64
	multipart bool
	br        *bufio.Reader
	crc       hash.Hash32
	byteCount int
}

type DecodeError string

func(d DecodeError) Error() string {
	return string(d)
}

//NewPart finds and parses the yEnc header in the reader and returns a
//part to use for further decoding.
func NewPart(r io.Reader) (*Part, error) {
	y := new(Part)

	y.br = bufio.NewReader(r)
	err := y.findHeader()
	if err != nil {
		return nil, err
	}

	err = y.parseHeader()
	if err != nil {
		return nil, err
	}
	y.crc = crc32.NewIEEE()
	return y, nil
}

//Read will read the content of a yEnc part, obeying the normal read rules
func (y *Part) Read(b []byte) (n int, err error) {
	for n < len(b) {
		var tok byte
		tok, err = y.br.ReadByte()
		if err != nil {
			if err == io.EOF {
				err = DecodeError("Unexpected End-of-File")
			}
			return
		}
		if tok == '\n' {
			continue
		}
		if tok == '=' {
			tok, err = y.br.ReadByte()
			if err != nil {
				if err == io.EOF {
					err = DecodeError("Unexpected End-of-File")
				}
				return
			}
			if tok == 'y' {
				y.crc.Write(b[:n])
				err = y.epilogue()
				return
			}
			tok -= 64
		}
		var c byte
		c = tok - 42
		b[n] = c
		n++
		y.byteCount++
	}
	y.crc.Write(b[:n])
	return
}

// handle footer and give an error if it fails crc.
func (y *Part) epilogue() error {
	footer, err := y.parseFooter()
	if err != nil {
		return err
	}

	if footer.size != y.byteCount {
		return DecodeError("Could not verify decoding: Sizes differ")
	}
	var crcp *uint32
	if y.multipart || footer.crc == 0 {
		crcp = &footer.pcrc
	} else {
		crcp = &footer.crc
	}

	if *crcp != y.crc.Sum32() {
		return DecodeError("Could not verify decoding: Bad CRC")
	}
	return io.EOF
}

func (y *Part) findHeader() error {
	const (
		StatePotential = iota
		StateNormal
	)
	i := 0
	str := "=ybegin "
	//regexp package will read past the end of the match, so making my own little matching statemachine
	state := StatePotential
	for {
		//when completely matched
		if i == len(str) {
			return nil
		}
		c, err := y.br.ReadByte()
		if err != nil {
			return DecodeError("Could not find header")
		}
		switch state {
		case StatePotential:
			if str[i] == c {
				i++
				continue
			} else if c != '\n' {
				state = StateNormal
			}
			i = 0
		case StateNormal:
			if c == '\n' {
				state = StatePotential
			}
		}
	}
	panic("unreachable")
}

func (y *Part) parseHeader() error {
	err := y.parseDataline()
	if err != nil {
		return err
	}
	//dealing with single part. don't handle partline
	if y.NumParts == 0 && y.Number == 0 {
		return nil
	}
	err = y.parsePartline()
	y.Size = y.end - y.Begin
	y.multipart = true

	return err
}

func (y *Part) parseDataline() error {
	dline, err := y.br.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			err = DecodeError("Unexpected End-of-File")
		}
		return err
	}

	dline = dline[:len(dline)-1]
	dbuf := bytes.NewBufferString(dline)

	for {
		name, err := consumeName(dbuf)
		if err != nil {
			return DecodeError("Malformed header")
		}

		if name == "name" {
			break
		}
		value, err := consumeValue(dbuf)
		if err != nil {
			return DecodeError("Malformed header")
		}

		err = y.handleAttrib(name, value)
		if err != nil {
			return DecodeError("Malformed header")
		}
	}
	y.Filename = dbuf.String()
	return nil
}

func (y *Part) parsePartline() error {
	//move past =ypart
	_, err := y.br.ReadString(' ')
	if err != nil {
		if err == io.EOF {
			err = DecodeError("Unexpected End-of-File")
		}
		return err
	}

	pline, err := y.br.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			err = DecodeError("Unexpected End-of-File")
		}
		return err
	}

	pline = pline[:len(pline)-1]
	pbuf := bytes.NewBufferString(pline)
	var name, value string
	for {
		name, err = consumeName(pbuf)
		if err != nil {
			return DecodeError("Malformed header")
		}

		value, err = consumeValue(pbuf)
		if err == io.EOF {
			break
		} else if err != nil {
			return DecodeError("Malformed header")
		}

		err = y.handleAttrib(name, value)
		if err != nil {
			return DecodeError("Malformed header")
		}
	}
	//handle the last value through the loop
	err = y.handleAttrib(name, value)
	if err != nil {
		return DecodeError("Malformed header")
	}
	return nil
}

func (y *Part) handleAttrib(name, value string) error {
	var err error
	switch name {
	case "line":
		//ignore because noone actually cares
	case "size":
		if y.Size == 0 {
			_, err = fmt.Sscan(value, &y.Size)
		}
	case "part":
		//noone cares
		_, err = fmt.Sscan(value, &y.Number)
	case "total":
		_, err = fmt.Sscan(value, &y.NumParts)
	case "begin":
		_, err = fmt.Sscan(value, &y.Begin)
		y.Begin--
	case "end":
		_, err = fmt.Sscan(value, &y.end)
	default:
		err = fmt.Errorf("Unknown Attribute: %s=%s", name, value)
	}
	return err
}

type footer struct {
	size int
	crc  uint32
	pcrc uint32
}

// Parse the footer of a yenc part
// we can sorta handle a corrupted footer
// so instead of dumping out, return the error
func (y *Part) parseFooter() (*footer, error) {
	f := new(footer)
	// move past =yend
	// really, we've only checked for "=y"
	// but in practice this works.
	_, err := y.br.ReadString(' ')
	if err != nil {
		if err == io.EOF {
			err = DecodeError("Unexpected End-of-File")
		}
		return nil, err
	}

	// carve out a line to read from
	fline, err := y.br.ReadString('\n')
	if err != nil {
		// EOF is fine. just means that someone didn't end this line with \n
		// all other errors are not fine
		if err != io.EOF {
			return nil, err
		}
	}

	fbuf := bytes.NewBufferString(fline)
	var name, value string
	for {
		name, err = consumeName(fbuf)
		if err != nil {
			// if someone added whitespace to the end of this line, this will fail with an EOF
			// surface this to the callers
			if err == io.EOF {
				err = nil
			}
			return f, err
		}
		value, err = consumeValue(fbuf)
		if err != nil {
			if err != io.EOF {
				return f, DecodeError("Corrupt footer")
			}
		}

		err = f.handleAttrib(name, value)
		if err != nil {
			return f, DecodeError("Corrupt footer")
		}

	}
}

func (f *footer) handleAttrib(name, value string) error {
	var err error
	switch name {
	case "size":
		_, err = fmt.Sscan(value, &f.size)
	case "pcrc32":
		_, err = fmt.Sscanf(value, "%x", &f.pcrc)
	case "crc32":
		_, err = fmt.Sscanf(value, "%x", &f.crc)
	case "part":
		//noone cares
	}
	return err
}

func consumeName(b *bytes.Buffer) (string, error) {
	name, err := b.ReadString('=')
	if err != nil {
		return name, err
	}
	name = name[:len(name)-1]
	return name, nil
}

func consumeValue(b *bytes.Buffer) (string, error) {
	value, err := b.ReadString(' ')
	if err != nil {
		return value, err
	}
	value = value[:len(value)-1]
	return value, nil
}
