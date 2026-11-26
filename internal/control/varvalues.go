package control

import "strings"

// VarValue is the value one service gives a version variable.
type VarValue struct{ Var, Service, Value string }

// DistinctVars lists each variable once in order of first appearance, with its values joined as "Var=a". Services
// with different values of one variable show as "Var=a (s1) / b (s2)". An empty value shows as "(unset)".
func DistinctVars(vs []VarValue) []string {
	return DistinctVarsWith(vs, func(name, value string) string { return name + "=" + value })
}

// DistinctVarsWith is DistinctVars with the rendering of a variable and its value text chosen by the caller.
func DistinctVarsWith(vs []VarValue, render func(name, value string) string) []string {
	var order []string
	byVar := map[string][]VarValue{}
	for _, v := range vs {
		if _, ok := byVar[v.Var]; !ok {
			order = append(order, v.Var)
		}
		byVar[v.Var] = append(byVar[v.Var], v)
	}
	out := make([]string, 0, len(order))
	for _, name := range order {
		out = append(out, render(name, valueText(byVar[name])))
	}
	return out
}

func valueText(group []VarValue) string {
	var values []string
	services := map[string][]string{}
	for _, v := range group {
		val := v.Value
		if val == "" {
			val = "(unset)"
		}
		if _, ok := services[val]; !ok {
			values = append(values, val)
		}
		services[val] = append(services[val], v.Service)
	}
	if len(values) == 1 {
		return values[0]
	}
	parts := make([]string, len(values))
	for i, val := range values {
		parts[i] = val + " (" + strings.Join(services[val], ", ") + ")"
	}
	return strings.Join(parts, " / ")
}
