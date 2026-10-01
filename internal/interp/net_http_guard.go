package interp

import "strings"

// httpPostGuard mirrors std/net/net_hosted.kg's Http.post validation so the
// accelerator fails with the native body's exact text, not net/url's.
func httpPostGuard(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") {
		return ""
	}
	if !strings.HasPrefix(rawURL, "http://") {
		return "unsupported URL scheme"
	}
	rest := rawURL[len("http://"):]
	authority, path := rest, "/"
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		authority, path = rest[:slash], rest[slash:]
	}
	if hasUnsafeHTTPByte(path) {
		return "path contains a control character or a space"
	}
	if hasUnsafeHTTPByte(authority) {
		return "host contains a control character or a space"
	}
	if strings.Contains(authority, "@") {
		return "userinfo in URL is not supported"
	}
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 {
			return "malformed IPv6 host"
		}
		after := authority[end+1:]
		if after != "" && !strings.HasPrefix(after, ":") {
			return "malformed IPv6 host"
		}
	}
	return ""
}

func hasUnsafeHTTPByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}
