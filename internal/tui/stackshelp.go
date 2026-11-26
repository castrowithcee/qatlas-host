package tui

import (
	"slices"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
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

// distinctVersionItems lists each version variable once, with the values its services give it.
func distinctVersionItems(images []control.ImageInfo) []versionItem {
	vals := make([]control.VarValue, len(images))
	for i, img := range images {
		vals[i] = control.VarValue{Var: img.Var, Service: img.Service, Value: img.Version}
	}
	current := control.DistinctVarsWith(vals, func(_, v string) string { return v })
	var items []versionItem
	seen := map[string]bool{}
	for _, img := range images {
		if seen[img.Var] {
			continue
		}
		seen[img.Var] = true
		cur := current[len(items)]
		if cur == "(unset)" {
			cur = ""
		}
		items = append(items, versionItem{variable: img.Var, image: img.Image, current: cur})
	}
	return items
}
