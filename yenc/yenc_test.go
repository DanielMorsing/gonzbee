package yenc_test

import (
	. "gonzbee/yenc"
	"io/ioutil"
	"reflect"
	"testing"
)

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("got error: %s , expected none", err.Error())
	}
}

func TestSinglepartDecode(t *testing.T) {
	dec, err := ioutil.ReadFile("testdata/encoded.txt")
	checkErr(t, err)

	exp, err := ioutil.ReadFile("testdata/expected.txt")
	checkErr(t, err)

	dec, yenc, err := Decode(dec)
	checkErr(t, err)

	if yenc.Name != "testfile.txt" {
		t.Errorf("Wrong filename. Expected \"testfile.txt\", got: \"%s\"", yenc.Name)
	}

	//check if it's the same as the expected value
	if !reflect.DeepEqual(dec, exp) {
		t.Errorf("binaries differ")
	}

}
