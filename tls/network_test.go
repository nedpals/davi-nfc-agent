package tls

import (
	"testing"
)

func TestGetLANIPs(t *testing.T) {
	ips, err := GetLANIPs()
	if err != nil {
		t.Fatalf("GetLANIPs failed: %v", err)
	}

	t.Logf("Found LAN IPs: %v", ips)

	// Should find at least something on most systems
	// (though this could fail in isolated containers)
	if len(ips) == 0 {
		t.Log("Warning: No LAN IPs found (may be expected in some environments)")
	}
}

func TestGetAllHosts(t *testing.T) {
	hosts, err := GetAllHosts()
	if err != nil {
		t.Fatalf("GetAllHosts failed: %v", err)
	}

	t.Logf("Hosts for certificate: %v", hosts)

	// Should always include localhost and 127.0.0.1
	if len(hosts) < 2 {
		t.Error("Expected at least localhost and 127.0.0.1")
	}

	hasLocalhost := false
	hasLoopback := false
	for _, h := range hosts {
		if h == "localhost" {
			hasLocalhost = true
		}
		if h == "127.0.0.1" {
			hasLoopback = true
		}
	}

	if !hasLocalhost {
		t.Error("Expected localhost in hosts")
	}
	if !hasLoopback {
		t.Error("Expected 127.0.0.1 in hosts")
	}
}
