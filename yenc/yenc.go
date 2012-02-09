//Package yenc gives a decoding interface for yEnc encoded binaries
package yenc

import (
	"bytes"
	"bufio"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
)

func panicOn(err interface{}) {
	if err != nil {
		panic(err)
	}
}

//YencInfo holds the information needed in order to save the decoded file
//in the right spot. It also holds the information needed in order to
//assemble multipart yenc encoded files.
type Part struct {
	Name      string
	Begin     int64
	Size      int64
	Parts     int
	br        *bufio.Reader
}

//bufio.Reader or bytes.Buffer are the most common types you will work on,
//so use them directly instead of wrapping by using this interface
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

//Decode will decode the content of a yEnc part. It returns the decoded
//contents, the information needed in order to assemble a multipart binary
//and an error, if any.
//
//Note that the error may be present, even though part of the file was
//decoded. This tends to happen if there were dropped bytes, bad hash, or
//badly formed yEnc footer.
func (y *Part) Decode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	byteCount := 0
	crc := crc32.NewIEEE()
	crcw := bufio.NewWriter(crc)
	for {
		tok, err := y.br.ReadByte()
		panicOn(err)
		if tok == '\n' {
			continue
		}
		if tok == '=' {
			tok, err = y.br.ReadByte()
			panicOn(err)
			if tok == 'y' {
				break
			}
			tok -= 64
		}
		var c byte
		c = tok - 42
		bw.WriteByte(c)
		crcw.WriteByte(c)
		byteCount++
	}
	bw.Flush()
	crcw.Flush()
	footer, err := y.parseFooter()
	if err != nil {
		return fmt.Errorf("Could not verify decoding: %s", err.Error())
	}

	if footer.size != byteCount {
		return errors.New("Could not verify decoding: Sizes differ")
	}
	var crcp *uint32
	if y.Parts > 1 || footer.crc == 0 {
		crcp = &footer.pcrc
	} else {
		crcp = &footer.crc
	}

	if *crcp != crc.Sum32() {
		return errors.New("Could not verify decoding: Bad CRC")
	}
	return nil
}

func (p *Part) findHeader() error {
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
		c, err := p.br.ReadByte()
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
	if y.Parts == 0 {
		return nil
	}
	err = y.parsePartline()

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
	y.Name = dbuf.String()
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
		panicOn(err)
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
	case "total":
		_, err = fmt.Sscan(value, &y.Parts)
	case "begin":
		_, err = fmt.Sscan(value, &y.Begin)
	case "end":
		if y.Begin != 0 {
			var end int64
			_, err = fmt.Sscan(value, &end)
			y.Size = end - y.Begin - 1
		}
	default:
		err = errors.New("Unknown Attribute")
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
