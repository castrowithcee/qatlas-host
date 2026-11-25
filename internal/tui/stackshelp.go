package tui

import (
	"slices"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

func appendDistinct(list []string, v string) []string {
	if v == "" || slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

func plural(t kittui.Translator, n int, one, many kittui.TextID) string {
	if n == 1 {
		return t.Lookup(one, n)
	}
	return t.Lookup(many, n)
}
