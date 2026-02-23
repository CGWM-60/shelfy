package fileserver

import "net/http"

func Authorized(r *http.Request, username, password string) bool {
	if username == "" && password == "" {
		return true
	}
	u, p, ok := r.BasicAuth()
	if !ok {
		return false
	}
	return u == username && p == password
}
