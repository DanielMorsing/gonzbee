//Package yenc gives a decoding interface for yEnc encoded binaries
package yenc

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"regexp"
	"strings"
)

type decoder struct {
	buf *bytes.Buffer
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

//YencInfo holds the information needed in order to save the decoded file
//in the right spot. It also holds the information needed in order to
//assemble multipart yenc encoded files.
type YencInfo struct {
	MultiPart bool
	Name      string
	Begin     int64
	Size      int64
}

//Decode will decode the content of a yEnc part. It returns the decoded
//contents, the information needed in order to assemble a multipart binary
//and an error, if any.
//
//Note that the error may be present, even though part of the file was
//decoded. This tends to happen if there were dropped bytes, bad hash, or
//badly formed yEnc footer.
func Decode(part []byte) (decoded []byte, yenc *YencInfo, err error) {
	defer func() {
		if perr := recover(); perr != nil {
			err = perr.(error)
			err = fmt.Errorf("yEnc decode failed: %s", err.Error())
			decoded = nil
			yenc = nil
		}
	}()
	d := new(decoder)
	err = d.findHeader(part)
	if err != nil {
		return nil, nil, fmt.Errorf("yEnc decode failed: %s", err.Error())
	}
	yenc = new(YencInfo)
	header := d.parseHeader()

	yenc.Name = header.name
	yenc.Size = header.size

	if header.total > 1 {
		yenc.MultiPart = true
		yenc.Begin = header.begin - 1
	}

	buf := new(bytes.Buffer)
	crc := crc32.NewIEEE()

	byteCount := 0
	for {
		tok, err := d.buf.ReadByte()
		checkErr(err)
		if tok == '\n' {
			continue
		}
		if tok == '=' {
			tok, err := d.buf.ReadByte()
			checkErr(err)
			if tok == 'y' {
				break
			}
			tok -= 64
		}
		var c byte
		c = tok - 42
		buf.WriteByte(c)
		crc.Write([]byte{c})
		byteCount++
	}
	footer, err := d.parseFooter()
	if err != nil {
		return buf.Bytes(), yenc, fmt.Errorf("Could not verify decoding: %s", err.Error())
	}

	if footer.size != byteCount {
		return buf.Bytes(), yenc, errors.New("Could not verify decoding: Sizes differ")
	}
	var crcp *uint32
	if yenc.MultiPart {
		crcp = &footer.pcrc
	} else {
		crcp = &footer.crc
	}

	if *crcp != crc.Sum32() {
		return buf.Bytes(), yenc, errors.New("Could not verify decoding: Bad CRC")
	}
	return buf.Bytes(), yenc, nil
}

var headerRegexp = regexp.MustCompile("^=ybegin ")

func (d *decoder) findHeader(b []byte) error {
	i := headerRegexp.FindIndex(b)

	if i == nil {
		return errors.New("Could not find header")
	}
	d.buf = bytes.NewBuffer(b[i[1]:])
	return nil
}

type header struct {
	name  string
	size  int64
	part  int
	total int
	begin int64
	end   int64
}

func (d *decoder) parseHeader() *header {
	h := new(header)
	d.parseDataline(h)
	//dealing with single part. don't handle partline
	if h.total == 0 {
		return h
	}

	d.parsePartline(h)

	return h
}

func (d *decoder) parseDataline(h *header) {
	dline, err := d.buf.ReadString('\n')
	checkErr(err)

	dline = strings.TrimRight(dline, "\n")
	dbuf := bytes.NewBufferString(dline)

	for {
		name, err := consumeName(dbuf)
		checkErr(err)
		if name == "name" {
			break
		}
		value, err := consumeValue(dbuf)
		checkErr(err)

		err = h.handleAttrib(name, value)
		checkErr(err)
	}
	h.name = dbuf.String()
}

func (d *decoder) parsePartline(h *header) {
	//move past =ypart
	_, err := d.buf.ReadString(' ')
	checkErr(err)

	pline, err := d.buf.ReadString('\n')
	checkErr(err)

	pline = strings.TrimRight(pline, "\n")
	pbuf := bytes.NewBufferString(pline)
	var name, value string
	for {
		name, err = consumeName(pbuf)
		checkErr(err)
		value, err = consumeValue(pbuf)
		if err == io.EOF {
			break
		}
		checkErr(err)

		err = h.handleAttrib(name, value)
		checkErr(err)
	}
	//handle the last value through the loop
	err = h.handleAttrib(name, value)
	checkErr(err)
}

func (h *header) handleAttrib(name, value string) error {
	var err error
	switch name {
	case "line":
		//ignore because noone actually cares
	case "size":
		_, err = fmt.Sscan(value, &h.size)
	case "part":
		_, err = fmt.Sscan(value, &h.part)
	case "total":
		_, err = fmt.Sscan(value, &h.total)
	case "begin":
		_, err = fmt.Sscan(value, &h.begin)
	case "end":
		_, err = fmt.Sscan(value, &h.end)
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
func (d *decoder) parseFooter() (*footer, error) {
	corrupt := errors.New("Corrupted footer")
	f := new(footer)
	//move past =yend
	_, err := d.buf.ReadString(' ')
	if err != nil {
		return f, corrupt
	}

	fline, err := d.buf.ReadString('\n')
	if err != nil {
		return f, corrupt
	}

	fline = strings.TrimRight(fline, "\n")
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
