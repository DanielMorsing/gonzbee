//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

//Package nzb provides a function for parsing NZB files.
package nzb

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

//Segment represents a single segment in a file.
type Segment struct {
	//The amount of bytes
	Bytes int `xml:"bytes,attr"`
	//The sequence number
	Number int `xml:"number,attr"`
	//The Message-ID
	MsgId string `xml:",chardata"`
}

//File represents a single file in the NZB file.
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
	Segments []*Segment `xml:"segments>segment"`
}

//Nzb represents the top level for a NZB file
//It's just a dumb struct to contain all the files.
type Nzb struct {
	//The files described in the NZB file
	File []*File `xml:"file"`
}

//Parse parses an nzb document from the reader and returns
//a Nzb struct and an error if any.
func Parse(r io.Reader) (n *Nzb, err error) {
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
	if strings.ToLower(charset) != "iso-8859-1" {
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
