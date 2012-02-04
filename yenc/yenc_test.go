package yenc_test

import(
	. "gonzbee/yenc"
	"testing"
	"io/ioutil"
	"reflect"
)

func TestSinglepartDecode(t *testing.T) {
	dec, err := ioutil.ReadFile("testdata/encoded.txt")
	exp, err := ioutil.ReadFile("testdata/expected.txt")
	if err != nil {
		t.Errorf("got error: %s , expected none", err.Error())
	}

	dec, yenc, err := Decode(dec)
	if err != nil {
		t.Errorf("got error: %s , expected none", err.Error())
	}

	if yenc.Name != "testfile.txt" {
		t.Errorf("Wrong filename. Expected \"testfile.txt\", got: \"%s\"", yenc.Name)
	}

	//check if it's the same as the expected value
	if !reflect.DeepEqual(dec, exp) {
		t.Errorf("binaries differ")
	}

}
