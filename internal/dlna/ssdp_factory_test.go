package dlna

import "testing"

func TestShouldUseSSDPMulticastListen(t *testing.T) {
	cases := []struct {
		name    string
		network string
		address string
		want    bool
	}{
		{name: "udp4 short port", network: "udp4", address: ":1900", want: true},
		{name: "udp4 wildcard host", network: "udp4", address: "0.0.0.0:1900", want: true},
		{name: "udp4 explicit localhost", network: "udp4", address: "127.0.0.1:1900", want: false},
		{name: "udp4 wrong port", network: "udp4", address: ":1901", want: false},
		{name: "udp6 ignored", network: "udp6", address: ":1900", want: false},
		{name: "invalid address", network: "udp4", address: "1900", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldUseSSDPMulticastListen(tc.network, tc.address)
			if got != tc.want {
				t.Fatalf("shouldUseSSDPMulticastListen(%q, %q) = %v, want %v", tc.network, tc.address, got, tc.want)
			}
		})
	}
}
