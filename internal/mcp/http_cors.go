package mcp

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// validProtocolVersion accepts omitted versions and MCP versions supported here.
func validProtocolVersion(r *http.Request) bool {
	version := r.Header.Get(mcpProtocolHeader)
	return version == "" || version == "2025-03-26" || version == protocolVersion
}

// setStreamableHeaders writes transport headers shared by all /mcp responses.
func setStreamableHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(mcpProtocolHeader, protocolVersion)
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
	w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)
	w.Header().Add("Vary", "Origin")
	w.Header().Add("Vary", "Access-Control-Request-Method")
	w.Header().Add("Vary", "Access-Control-Request-Headers")
}

// validOrigin allows same-host and loopback browser origins.
func validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	originHost := hostWithoutPort(u.Host)
	requestHost := hostWithoutPort(r.Host)
	if originHost == "" {
		return false
	}
	return originHost == requestHost || isLoopbackHost(originHost)
}

// hostWithoutPort strips a port from a host while preserving IPv6 literals.
func hostWithoutPort(hostport string) string {
	if hostport == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(hostport)
	if err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(hostport, "[]")
}

// isLoopbackHost reports whether host names a local-only interface.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
