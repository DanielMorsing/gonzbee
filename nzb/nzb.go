//Package nzb provides a function for parsing NZB files
package nzb

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

//Segment represents a single segment in a file
type Segment struct {
	//The amount of bytes
	Bytes int `xml:"bytes,attr"`
	//The sequence number
	Number int `xml:"number,attr"`
	//The Message-ID
	MsgId string `xml:",innerxml"`
}

//File represents a single file in the NZB file
type File struct {
	//The person who posted this to usenet
	Poster string `xml:"poster,attr"`
	//The date of the post in unix time
	Date int `xml:"date,attr"`
	//A subject line. This normally contains the filename
	Subject string `xml:"subject,attr"`
	//Which groups it was posted on
	Groups []string `xml:"groups>group"`
	//The Segments comprising this file
	Segments []Segment `xml:"segments>segment"`
}

//Nzb represents the top level for a NZB file
//It's just a dumb struct to contain all the files
type Nzb struct {
	//The files described in the NZB file
	File []File `xml:"file"`
}

type charsetReader struct {
	reader    *bufio.Reader
	remainder []byte
}

func newCharsetReader(input io.Reader) *charsetReader {
	r := &charsetReader{reader: bufio.NewReader(input)}
	return r

}

func (c *charsetReader) Read(b []byte) (n int, err error) {
	//TODO: This is broken for reads smaller than utf8.UTFMax
	if c.remainder != nil {
		n = copy(b, c.remainder)
		c.remainder = nil
	}
	bs := b[n:]
	bsLen := len(bs)
	var char byte
	runeBuf := make([]byte, utf8.UTFMax)
	for bi := 0; bi < bsLen; {
		char, err = c.reader.ReadByte()
		if err != nil {
			return n, err
		}
		rSize := utf8.EncodeRune(runeBuf, rune(char))
		if bi+rSize > bsLen {
			c.remainder = runeBuf[:rSize]
			return n, err
		}
		copy(bs[bi:], runeBuf[:rSize])
		n += rSize
		bi += rSize
	}
	return n, err
}

//nzb files are almost always in iso8859-1, so provide a converter so that
//go's utf-8 world can handle it
func charsetter(charset string, input io.Reader) (r io.Reader, err error) {
	if charset != "iso-8859-1" {
		err = errors.New("Cannot handle charset")
		return
	}
	r = newCharsetReader(input)
	return r, err
}

func validate(n *Nzb) error {
	if len(n.File) < 1 {
		return errors.New("No files contained")
	}
	for _, file := range n.File {
		if len(file.Segments) < 1 {
			return errors.New("Zero segment file")
		}
	}
	return nil
}

//Parse Nzb returns a pointer to a filled in Nzb struct, with the xml
//document that it reads from r.
//If there is an error, n will be nil and err will be non-nil
func ParseNzb(r io.Reader) (n *Nzb, err error) {
	parser := xml.NewDecoder(r)
	parser.CharsetReader = charsetter

	n = new(Nzb)
	err = parser.DecodeElement(n, nil)
	if err != nil {
		err = errors.New(fmt.Sprintf("Could not parse NZB XML: %s", err.Error()))
		return nil, err
	}
	err = validate(n)
	if err != nil {
		err = errors.New(fmt.Sprintf("Invalid NZB file: %s", err.Error()))
		return nil, err
	}
	return n, nil
}
