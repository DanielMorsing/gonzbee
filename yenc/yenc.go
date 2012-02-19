//Package yenc gives a decoding interface for yEnc encoded binaries.
package yenc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
)

//Part holds the information contained in a parsed yEnc header.
//It also provides an interface to decoding the data in it.
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
	return y, nil
}

//Decode will decode the content of a yEnc part and write it to the passed writer. 
func (y *Part) Decode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	byteCount := 0
	crc := crc32.NewIEEE()
	crcw := bufio.NewWriter(crc)
	for {
		tok, err := y.br.ReadByte()
		if err != nil {
			return errors.New("Unexpected End-of-File")
		}
		if tok == '\n' {
			continue
		}
		if tok == '=' {
			tok, err = y.br.ReadByte()
			if err != nil {
				return errors.New("Unexpected End-of-File")
			}
			if tok == 'y' {
				break
			}
			tok -= 64
		}
		var c byte
		c = tok - 42
		err = bw.WriteByte(c)
		if err != nil {
			return errors.New("I/O Error")
		}
		crcw.WriteByte(c)
		byteCount++
	}
	err := bw.Flush()
	if err != nil {
		return errors.New("I/O Error")
	}
	crcw.Flush()
	footer, err := y.parseFooter()
	if err != nil {
		return fmt.Errorf("Could not verify decoding: %s", err.Error())
	}

	if footer.size != byteCount {
		return errors.New("Could not verify decoding: Sizes differ")
	}
	var crcp *uint32
	if y.multipart || footer.crc == 0 {
		crcp = &footer.pcrc
	} else {
		crcp = &footer.crc
	}

	if *crcp != crc.Sum32() {
		return errors.New("Could not verify decoding: Bad CRC")
	}
	return nil
}

func (y *Part) findHeader() error {
	const (
		StatePotential = iota
		StateNormal
	)
	i := 0
	str := []byte("=ybegin ")
	//regexp package will read past the end of the match, so making my own little matching statemachine
	state := StatePotential
	for {
		//when completely matched
		if i == len(str) {
			return nil
		}
		c, err := y.br.ReadByte()
		if err != nil {
			return errors.New("Could not find header")
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
		return errors.New("Malformed Header")
	}

	dline = strings.TrimRight(dline, "\n")
	dbuf := bytes.NewBufferString(dline)

	for {
		name, err := consumeName(dbuf)
		if err != nil {
			return errors.New("Malformed Header")
		}

		if name == "name" {
			break
		}
		value, err := consumeValue(dbuf)
		if err != nil {
			return errors.New("Malformed Header")
		}

		err = y.handleAttrib(name, value)
		if err != nil {
			return errors.New("Malformed yEnc Attribute")
		}
	}
	y.Filename = dbuf.String()
	return nil
}

func (y *Part) parsePartline() error {
	//move past =ypart
	_, err := y.br.ReadString(' ')
	if err != nil {
		return errors.New("Malformed Header")
	}

	pline, err := y.br.ReadString('\n')
	if err != nil {
		return errors.New("Malformed Header")
	}

	pline = strings.TrimRight(pline, "\n")
	pbuf := bytes.NewBufferString(pline)
	var name, value string
	for {
		name, err = consumeName(pbuf)
		if err != nil {
			return errors.New("Malformed Header")
		}

		value, err = consumeValue(pbuf)
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.New("Malformed Header")
		}

		err = y.handleAttrib(name, value)
		if err != nil {
			return errors.New("Malformed Header")
		}
	}
	//handle the last value through the loop
	err = y.handleAttrib(name, value)
	return err
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

//we can sorta handle a corrupted footer
//so instead of dumping out, return the error
func (y *Part) parseFooter() (*footer, error) {
	corrupt := errors.New("Corrupted footer")
	f := new(footer)
	//move past =yend
	_, err := y.br.ReadString(' ')
	if err != nil {
		return f, corrupt
	}

	fline, err := y.br.ReadString('\n')
	if err != nil {
		return f, corrupt
	}

	fline = strings.TrimRight(fline, " \n")
	fbuf := bytes.NewBufferString(fline)
	var name, value string
	for {
		name, err = consumeName(fbuf)
		if err != nil {
			return f, err
		}
		value, err = consumeValue(fbuf)
		if err != nil && err == io.EOF {
			break
		} else if err != nil {
			return f, corrupt
		}

		err = f.handleAttrib(name, value)
		if err != nil {
			return f, corrupt
		}

	}
	//handle the last value through the loop
	err = f.handleAttrib(name, value)
	if err != nil {
		return f, corrupt
	}
	return f, nil
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
	name = strings.TrimRight(name, "=")
	return name, nil
}

func consumeValue(b *bytes.Buffer) (string, error) {
	value, err := b.ReadString(' ')
	if err != nil {
		return value, err
	}
	value = strings.TrimRight(value, " ")
	return value, nil
}
