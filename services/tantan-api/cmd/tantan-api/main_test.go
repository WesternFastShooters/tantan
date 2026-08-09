package main

import "testing"

func TestResolveServerURLAllowsOnlyOfficialOrLoopback(t *testing.T) {
	for _, test := range []struct {
		name         string
		raw          string
		officialHost string
		want         string
		wantError    bool
	}{
		{name: "default API", officialHost: "api.folo.is", want: "https://api.folo.is"},
		{name: "official web", raw: "https://app.folo.is/", officialHost: "app.folo.is", want: "https://app.folo.is"},
		{name: "loopback IPv4", raw: "http://127.0.0.1:4100", officialHost: "api.folo.is", want: "http://127.0.0.1:4100"},
		{name: "loopback localhost", raw: "http://localhost:4100", officialHost: "app.folo.is", want: "http://localhost:4100"},
		{name: "external HTTPS", raw: "https://api.attacker.invalid", officialHost: "api.folo.is", wantError: true},
		{name: "lookalike host", raw: "https://api.folo.is.attacker.invalid", officialHost: "api.folo.is", wantError: true},
		{name: "credentials", raw: "https://user:secret@api.folo.is", officialHost: "api.folo.is", wantError: true},
		{name: "query", raw: "https://api.folo.is?next=https://attacker.invalid", officialHost: "api.folo.is", wantError: true},
		{name: "loopback path", raw: "http://127.0.0.1:4100/api", officialHost: "api.folo.is", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := resolveServerURL(test.raw, "https://"+test.officialHost, test.officialHost)
			if test.wantError {
				if err == nil {
					t.Fatalf("accepted unsafe URL %q", test.raw)
				}
				return
			}
			if err != nil || value.String() != test.want {
				t.Fatalf("value=%v err=%v", value, err)
			}
		})
	}
}
