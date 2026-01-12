package shell

import "strings"

func ctxGet(st *State, key string) string {
	if st == nil || st.Ctx == nil {
		return ""
	}
	return strings.TrimSpace(st.Ctx[key])
}
func ctxSet(st *State, key, val string) {
	if st == nil {
		return
	}
	if st.Ctx == nil {
		st.Ctx = map[string]string{}
	}
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if key == "" {
		return
	}
	st.Ctx[key] = val
}
func ctxClear(st *State) {
	if st != nil {
		st.Ctx = map[string]string{}
	}
}
