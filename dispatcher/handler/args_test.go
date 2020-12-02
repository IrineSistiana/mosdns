package handler

import (
	"reflect"
	"testing"
)

type TestArgsStruct struct {
	A string `yaml:"1"`
	B []int  `yaml:"2"`
}

func TestArgs_WeakDecode(t *testing.T) {
	testObj := new(TestArgsStruct)
	testArgs := map[string]interface{}{
		"1": "test",
		"2": []int{1, 2, 3},
	}
	wantObj := &TestArgsStruct{
		A: "test",
		B: []int{1, 2, 3},
	}

	err := WeakDecode(testArgs, testObj)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(testObj, wantObj) {
		t.Fatalf("args decode failed, want %v, got %v", wantObj, testObj)
	}
}
