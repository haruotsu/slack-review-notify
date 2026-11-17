package handlers

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	runAsync = func(f func()) { f() }
	os.Exit(m.Run())
}
