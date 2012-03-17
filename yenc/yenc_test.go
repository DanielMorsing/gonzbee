package yenc_test

import (
	. "gonzbee/yenc"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("got error: %s , expected none", err.Error())
	}
}

type testYenc struct {
	begin  int64
	name   string
	size   int64
	number int
}

func checkPart(t *testing.T, p *Part, test *testYenc) {
	if p.Filename != test.name {
		t.Errorf("Wrong filename. Expected \"%s\", got: \"%s\"", test.name, p.Filename)
	}
	if p.Size != test.size {
		t.Errorf("Wrong size, Expected %d, got %d", test.size, p.Size)
	}
	if p.Begin != test.begin {
		t.Errorf("Wrong Begin, Expected %d, got %d", test.begin, p.Begin)
	}
	if p.Number != test.number {
		t.Errorf("Wrong part number, Expected %d, got %d", test.number, p.Number)
	}
	if t.Failed() {
		t.FailNow()
	}
}

func TestSinglepartDecode(t *testing.T) {
	dec, err := os.Open("testdata/encoded.txt")
	checkErr(t, err)
	defer dec.Close()

	yenc, err := NewPart(dec)
	checkErr(t, err)

	exp, err := ioutil.ReadFile("testdata/expected.txt")
	checkErr(t, err)

	data := testYenc{
		begin:  0,
		size:   int64(len(exp)),
		number: 0,
		name:   "testfile.txt",
	}

	checkPart(t, yenc, &data)

	buf, err := ioutil.ReadAll(yenc)
	checkErr(t, err)

	//check if it's the same as the expected value
	if !reflect.DeepEqual(buf, exp) {
		t.Errorf("binaries differ")
	}
}

func TestMultipartDecode(t *testing.T) {
	dec, err := os.Open("testdata/00000020.ntx")
	checkErr(t, err)
	defer dec.Close()

	yenc, err := NewPart(dec)
	data := testYenc{
		begin:  0,
		size:   11250,
		name:   "joystick.jpg",
		number: 1,
	}
	checkPart(t, yenc, &data)
	buf1, err := ioutil.ReadAll(yenc)
	checkErr(t, err)

	dec, err = os.Open("testdata/00000021.ntx")
	checkErr(t, err)
	defer dec.Close()
	data = testYenc{
		begin:  11250,
		size:   8088,
		name:   "joystick.jpg",
		number: 2,
	}

	yenc, err = NewPart(dec)
	checkPart(t, yenc, &data)
	buf2, err := ioutil.ReadAll(yenc)
	checkErr(t, err)

	exp, err := ioutil.ReadFile("testdata/joystick.jpg")
	checkErr(t, err)
	decoded := append(buf1, buf2...)
	if !reflect.DeepEqual(exp, decoded) {
		t.Errorf("binaries differ")
	}

}
