// Package tls provides automatic TLS certificate management with cross-platform trust store installation.
package tls

import (
	"net"
)

// GetLANIPs returns all local IPv4 addresses (non-loopback).
func GetLANIPs() ([]string, error) {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// Skip down or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only include IPv4 addresses
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ips = append(ips, ip.String())
			}
		}
	}

	return ips, nil
}

// GetAllHosts returns localhost + LAN IPs for certificate generation.
func GetAllHosts() ([]string, error) {
	hosts := []string{"localhost", "127.0.0.1"}

	lanIPs, err := GetLANIPs()
	if err != nil {
		return hosts, err
	}

	hosts = append(hosts, lanIPs...)
	return hosts, nil
}
