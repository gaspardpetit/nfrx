package ctrl

import "regexp"

var aliasRe = regexp.MustCompile(`^([^:]+):([^-\s]+)(-.+)?$`)

func AliasKey(id string) (string, bool) {
	m := aliasRe.FindStringSubmatch(id)
	if m == nil {
		return "", false
	}
	return m[1] + ":" + m[2], true
}
