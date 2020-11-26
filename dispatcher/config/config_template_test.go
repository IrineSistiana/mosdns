package config

import (
	"testing"
)

func Test_GetTemplateConfig(t *testing.T) {
	_, err := GetTemplateConfig()
	if err != nil {
		t.Fatal(err)
	}
}
