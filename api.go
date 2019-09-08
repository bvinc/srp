package srp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// APIHandler returns nil if the whitelisted subnet hasn't been configured.
// This design works well with autocert.HTTPHandler(fallback), which uses its
// own fallback if fallback is nil.
func (rp *ReverseProxy) APIHandler() (http.Handler, error) {
	reg := rp.cloneRegistry()
	if reg.API.Subnet == "" {
		return nil, nil
	}
	if localIP := getLocalIP(); localIP == "" {
		return nil, nil
	}
	maskedIP, mask, err := maskIP(reg.API.Subnet)
	if err != nil {
		return nil, fmt.Errorf("mask: %w", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET", "HEAD":
			http.Error(w, "Use HTTPS", http.StatusBadRequest)
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if maskedIP == net.ParseIP(host).Mask(mask).String() {
			if strings.TrimPrefix(r.URL.Path, "/") == "services" {
				err := json.NewEncoder(w).Encode(rp.cloneRegistry())
				if err != nil {
					log.Printf("failed to encode registry: %s", err)
				}
				return
			}
		}
		target := "https://" + stripPort(r.Host) + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusFound)
	}), nil
}

func stripPort(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return net.JoinHostPort(host, "443")
}

// getLocalIP returns the non-loopback local IP of the host.
//
// This is taken from https://stackoverflow.com/a/31551220
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the
		// display it
		ipnet, ok := address.(*net.IPNet)
		if ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func maskIP(subnet string) (string, net.IPMask, error) {
	parts := strings.SplitN(subnet, "/", 2)
	if len(parts) != 2 {
		return "", nil, errors.New("bad subnet: expected ip/mask in the form of 10.1.2.0/24")
	}
	maskBits, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("bad mask: %w", err)
	}
	mask := net.CIDRMask(maskBits, 32)
	ip := net.ParseIP(parts[0])
	maskedIP := ip.Mask(mask).String()
	if maskedIP == "<nil>" {
		return "", nil, fmt.Errorf("bad masked ip: %s", parts[0])
	}
	return maskedIP, mask, nil
}
