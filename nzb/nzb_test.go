//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

package nzb_test

import (
	. "github.com/DanielMorsing/gonzbee/nzb"
	"reflect"
	"strings"
	"testing"
)

var singleFile string = `<?xml version="1.0" encoding="iso-8859-1" ?>
<!DOCTYPE nzb PUBLIC "-//newzBin//DTD NZB 1.0//EN" "http://www.newzbin.com/DTD/nzb/nzb-1.0.dtd">
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
<file poster="Joe Example &lt;Joe@Example.com&gt;" date="2000000000" subject="Here is your file &quot;example.rar&quot; yEnc (1/1)">
<groups>
<group>alt.binaries.example</group>
</groups>
<segments>
<segment bytes="14043" number="1">4f08c1ce$0$32047$c3e8da3$853bf72e@news.astraweb.com</segment>
</segments>
</file>
</nzb>`

var invalid string = `<?xml version="1.0" encoding="iso-8859-1" ?>
<!DOCTYPE nzb PUBLIC "-//newzBin//DTD NZB 1.0//EN" "http://www.newzbin.com/DTD/nzb/nzb-1.0.dtd">
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
<file poster="Joe Example &lt;Joe@Example.com&gt;" date="2000000000" subject="Here is your file &quot;example.rar&quot; yEnc (1/1)">
<groups>
<group>alt.binaries.example</group>
</groups>
</file>
</nzb>`

var topnzb Nzb = Nzb{[]*File{
	{
		Poster:   "Joe Example <Joe@Example.com>",
		Date:     2000000000,
		Subject:  "Here is your file \"example.rar\" yEnc (1/1)",
		Groups:   []string{"alt.binaries.example"},
		Segments: []*Segment{{Bytes: 14043, Number: 1, MsgId: "4f08c1ce$0$32047$c3e8da3$853bf72e@news.astraweb.com"}},
	}}}

func checkResult(t *testing.T, expected interface{}, was interface{}, err error) {
	if err != nil {
		t.Error(err.Error())
		return
	}
	if !reflect.DeepEqual(was, expected) {
		t.Errorf("Expected: %v\nWas: %v", expected, was)
		return
	}
}

func TestMarshal(t *testing.T) {
	reader := strings.NewReader(singleFile)
	nzb, err := Parse(reader)
	checkResult(t, &topnzb, nzb, err)
}

func TestInvalidNzb(t *testing.T) {
	reader := strings.NewReader(invalid)
	nzb, err := Parse(reader)
	if err == nil {
		t.Errorf("Parsed an invalid NZB. Got: %v", nzb)
	}
}
