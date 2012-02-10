package yenc_test

import (
	"bytes"
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

func TestSinglepartDecode(t *testing.T) {
	dec, err := os.Open("testdata/encoded.txt")
	checkErr(t, err)
	defer dec.Close()

	yenc, err := NewPart(dec)
	checkErr(t, err)

	if yenc.Name != "testfile.txt" {
		t.Fatalf("Wrong filename. Expected \"testfile.txt\", got: \"%s\"", yenc.Name)
	}

	exp, err := ioutil.ReadFile("testdata/expected.txt")
	checkErr(t, err)

	buf := bytes.Buffer{}
	err = yenc.Decode(&buf)
	checkErr(t, err)

	//check if it's the same as the expected value
	if !reflect.DeepEqual(buf.Bytes(), exp) {
		t.Errorf("binaries differ")
	}

}
