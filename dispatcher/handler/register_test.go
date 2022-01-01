package handler

import (
	"testing"
)

func TestPluginRegister(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	makePlugin := func(tag string) Plugin {
		return &BP{tag: tag}
	}
	testBool := func(b, want bool) {
		if b != want {
			t.Fatal()
		}
	}

	testBool(RegPlugin(makePlugin("p1")), true)
	testBool(RegPlugin(makePlugin("p2")), true)
	testBool(RegPlugin(makePlugin("p2")), false)
	DelPlugin("p2")
	testBool(RegPlugin(makePlugin("p2")), true)
	testBool(GetPlugin("p1") != nil, true)
	testBool(GetPlugin("p2") != nil, true)
	testBool(GetPlugin("p3") != nil, false)
}
