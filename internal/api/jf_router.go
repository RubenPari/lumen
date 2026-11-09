package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Jellyfin-compatible API (subset used by the official and third-party
// clients). Jellyfin routes are case-insensitive and may be prefixed with
// /emby or /jellyfin, so a tiny dedicated router is used.

type jfRoute struct {
	method string
	parts  []string // lower-case; "{name}" captures
	h      handler
	auth   bool
}

type jfRouter struct {
	s      *Server
	routes []jfRoute
}

type jfParamsKey struct{}

func jfParam(r *http.Request, name string) string {
	m, _ := r.Context().Value(jfParamsKey{}).(map[string]string)
	return m[strings.ToLower(name)]
}

// jfQuery reads a query parameter case-insensitively.
func jfQuery(r *http.Request, name string) string {
	for k, v := range r.URL.Query() {
		if strings.EqualFold(k, name) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func (jr *jfRouter) add(method, pattern string, auth bool, h handler) {
	var parts []string
	for _, p := range strings.Split(strings.Trim(pattern, "/"), "/") {
		if !strings.HasPrefix(p, "{") {
			p = strings.ToLower(p)
		} else {
			p = strings.ToLower(p)
		}
		parts = append(parts, p)
	}
	jr.routes = append(jr.routes, jfRoute{method: method, parts: parts, h: h, auth: auth})
}

func (jr *jfRouter) get(p string, h handler)  { jr.add("GET", p, true, h) }
func (jr *jfRouter) post(p string, h handler) { jr.add("POST", p, true, h) }
func (jr *jfRouter) del(p string, h handler)  { jr.add("DELETE", p, true, h) }

// match finds a route; it returns nil when the path is not a Jellyfin route.
func (jr *jfRouter) match(r *http.Request) (*jfRoute, map[string]string, bool) {
	path := strings.Trim(r.URL.Path, "/")
	lower := strings.ToLower(path)
	for _, pre := range []string{"emby/", "jellyfin/", "mediabrowser/"} {
		if strings.HasPrefix(lower, pre) {
			path, lower = path[len(pre):], lower[len(pre):]
		}
	}
	segs := strings.Split(path, "/")
	lsegs := strings.Split(lower, "/")
	pathMatched := false
	var best *jfRoute
	var bestParams map[string]string
	bestScore := -1
	for i := range jr.routes {
		rt := &jr.routes[i]
		if len(rt.parts) != len(segs) {
			continue
		}
		params := map[string]string{}
		score := 0
		ok := true
		for j, p := range rt.parts {
			if strings.HasPrefix(p, "{") {
				params[strings.Trim(p, "{}")] = segs[j]
				continue
			}
			if p != lsegs[j] {
				ok = false
				break
			}
			score++
		}
		if !ok {
			continue
		}
		pathMatched = true
		methodOK := rt.method == r.Method || (rt.method == "GET" && r.Method == http.MethodHead)
		// Literal segments win over parameters ("/Items/Latest" over "/Items/{id}").
		if methodOK && score > bestScore {
			best, bestParams, bestScore = rt, params, score
		}
	}
	return best, bestParams, pathMatched
}

// ServeHTTP handles a request if it matches a Jellyfin route. It returns
// false when the request should be handled by someone else.
func (jr *jfRouter) serve(w http.ResponseWriter, r *http.Request) bool {
	rt, params, pathMatched := jr.match(r)
	if rt == nil {
		if pathMatched && r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		if pathMatched {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		return false
	}
	r = r.WithContext(context.WithValue(r.Context(), jfParamsKey{}, params))
	h := rt.h
	if rt.auth {
		h = jr.s.jfRequireUser(h)
	}
	jr.s.wrapJF(h)(w, r)
	return true
}

// wrapJF is like wrap but writes plain errors as Jellyfin does.
func (s *Server) wrapJF(h handler) http.HandlerFunc {
	inner := s.wrap(h)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		inner(w, r)
	}
}

func jfJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(v)
}

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// cors handles preflight requests for Jellyfin web-based clients.
func jfCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions && !strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Emby-Authorization, X-Emby-Token, X-MediaBrowser-Token, Range")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
