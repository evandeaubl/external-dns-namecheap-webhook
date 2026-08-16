package webhook

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSplitDomain(t *testing.T) {
	client := &NamecheapClient{
		tlds: []string{"co.uk", "com", "net", "org"},
		tldSet: map[string]bool{
			"co.uk": true,
			"com":   true,
			"net":   true,
			"org":   true,
		},
	}

	tests := []struct {
		domain         string
		expectedSLD    string
		expectedTLD    string
		expectedHost   string
	}{
		{"example.com", "example", "com", "@"},
		{"www.example.com", "example", "com", "www"},
		{"a.b.example.com", "example", "com", "a.b"},
		{"example.co.uk", "example", "co.uk", "@"},
		{"www.example.co.uk", "example", "co.uk", "www"},
		{"example.net", "example", "net", "@"},
		{"example.org", "example", "org", "@"},
		{"*.example.com", "example", "com", "*"},
	}

	for _, tt := range tests {
		sld, tld, host := client.SplitDomain(tt.domain)
		if sld != tt.expectedSLD || tld != tt.expectedTLD || host != tt.expectedHost {
			t.Errorf("SplitDomain(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.domain, sld, tld, host, tt.expectedSLD, tt.expectedTLD, tt.expectedHost)
		}
	}
}

func TestExtractDomain(t *testing.T) {
	client := &NamecheapClient{
		tlds: []string{"co.uk", "com", "net", "org"},
		tldSet: map[string]bool{
			"co.uk": true,
			"com":   true,
			"net":   true,
			"org":   true,
		},
	}
	srv := &Server{client: client, cfg: &Config{}}

	tests := []struct {
		dnsName  string
		expected string
	}{
		{"www.example.com", "example.com"},
		{"example.com", "example.com"},
		{"a.b.example.com", "example.com"},
		{"www.example.co.uk", "example.co.uk"},
		{"example.co.uk", "example.co.uk"},
		{"test.example.net", "example.net"},
	}

	for _, tt := range tests {
		result := srv.extractDomain(tt.dnsName)
		if result != tt.expected {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.dnsName, result, tt.expected)
		}
	}
}

func TestEndpointHostname(t *testing.T) {
	srv := &Server{}

	tests := []struct {
		dnsName    string
		domainName string
		expected   string
	}{
		{"www.example.com", "example.com", "www"},
		{"example.com", "example.com", "@"},
		{"a.b.example.com", "example.com", "a.b"},
		{"www.example.com.", "example.com", "www"},
		{"*.example.com", "example.com", "*"},
		{"test", "example.com", "test"},
	}

	for _, tt := range tests {
		result := srv.endpointHostname(tt.dnsName, tt.domainName)
		if result != tt.expected {
			t.Errorf("endpointHostname(%q, %q) = %q, want %q",
				tt.dnsName, tt.domainName, result, tt.expected)
		}
	}
}

func TestHostToEndpoint(t *testing.T) {
	srv := &Server{}

	tests := []struct {
		name       string
		host       Host
		domainName string
		expected   *Endpoint
	}{
		{
			name: "A record at apex",
			host: Host{Name: "@", Type: "A", Address: "1.2.3.4", TTL: "1800"},
			domainName: "example.com",
			expected: &Endpoint{
				DNSName: "example.com",
				RecordType: "A",
				RecordTTL: 1800,
				Targets: []string{"1.2.3.4"},
			},
		},
		{
			name: "A record subdomain",
			host: Host{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "300"},
			domainName: "example.com",
			expected: &Endpoint{
				DNSName: "www.example.com",
				RecordType: "A",
				RecordTTL: 300,
				Targets: []string{"1.2.3.4"},
			},
		},
		{
			name: "CNAME record",
			host: Host{Name: "www", Type: "CNAME", Address: "target.example.org", TTL: "3600"},
			domainName: "example.com",
			expected: &Endpoint{
				DNSName: "www.example.com",
				RecordType: "CNAME",
				RecordTTL: 3600,
				Targets: []string{"target.example.org"},
			},
		},
		{
			name: "MX record with preference",
			host: Host{Name: "@", Type: "MX", Address: "mail.example.com", MXPref: "10", TTL: "1800"},
			domainName: "example.com",
			expected: &Endpoint{
				DNSName: "example.com",
				RecordType: "MX",
				RecordTTL: 1800,
				Targets: []string{"10 mail.example.com"},
			},
		},
		{
			name: "TXT record",
			host: Host{Name: "@", Type: "TXT", Address: "v=spf1 include:_spf.example.com ~all", TTL: "3600"},
			domainName: "example.com",
			expected: &Endpoint{
				DNSName: "example.com",
				RecordType: "TXT",
				RecordTTL: 3600,
				Targets: []string{"v=spf1 include:_spf.example.com ~all"},
			},
		},
		{
			name: "URL redirect skipped",
			host: Host{Name: "@", Type: "URL", Address: "http://example.com", TTL: "1800"},
			domainName: "example.com",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := srv.hostToEndpoint(tt.host, tt.domainName)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected non-nil result")
			}
			if result.DNSName != tt.expected.DNSName {
				t.Errorf("DNSName = %q, want %q", result.DNSName, tt.expected.DNSName)
			}
			if result.RecordType != tt.expected.RecordType {
				t.Errorf("RecordType = %q, want %q", result.RecordType, tt.expected.RecordType)
			}
			if result.RecordTTL != tt.expected.RecordTTL {
				t.Errorf("RecordTTL = %d, want %d", result.RecordTTL, tt.expected.RecordTTL)
			}
			if len(result.Targets) != len(tt.expected.Targets) || result.Targets[0] != tt.expected.Targets[0] {
				t.Errorf("Targets = %v, want %v", result.Targets, tt.expected.Targets)
			}
		})
	}
}

func TestApplyEndpointToHosts(t *testing.T) {
	srv := &Server{}

	t.Run("A record", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep := &Endpoint{
			DNSName:    "www.example.com",
			RecordType: "A",
			RecordTTL:  300,
			Targets:    []string{"1.2.3.4"},
		}
		srv.applyEndpointToHosts(hostMap, ep, "example.com")

		hosts := hostMap["www"]
		if len(hosts) != 1 {
			t.Fatalf("expected 1 host, got %d", len(hosts))
		}
		host := hosts[0]
		if host.Type != "A" {
			t.Errorf("Type = %q, want %q", host.Type, "A")
		}
		if host.Address != "1.2.3.4" {
			t.Errorf("Address = %q, want %q", host.Address, "1.2.3.4")
		}
		if host.TTL != "300" {
			t.Errorf("TTL = %q, want %q", host.TTL, "300")
		}
	})

	t.Run("MX record", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep := &Endpoint{
			DNSName:    "example.com",
			RecordType: "MX",
			RecordTTL:  1800,
			Targets:    []string{"10 mail.example.com"},
		}
		srv.applyEndpointToHosts(hostMap, ep, "example.com")

		hosts := hostMap["@"]
		if len(hosts) != 1 {
			t.Fatalf("expected 1 host, got %d", len(hosts))
		}
		host := hosts[0]
		if host.MXPref != "10" {
			t.Errorf("MXPref = %q, want %q", host.MXPref, "10")
		}
		if host.Address != "mail.example.com" {
			t.Errorf("Address = %q, want %q", host.Address, "mail.example.com")
		}
	})

	t.Run("TTL clamping", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep := &Endpoint{
			DNSName:    "www.example.com",
			RecordType: "A",
			RecordTTL:  10,
			Targets:    []string{"1.2.3.4"},
		}
		srv.applyEndpointToHosts(hostMap, ep, "example.com")

		host := hostMap["www"][0]
		if host.TTL != "60" {
			t.Errorf("TTL = %q, want %q (clamped to min)", host.TTL, "60")
		}
	})

	t.Run("TTL zero uses default", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep := &Endpoint{
			DNSName:    "www.example.com",
			RecordType: "A",
			RecordTTL:  0,
			Targets:    []string{"1.2.3.4"},
		}
		srv.applyEndpointToHosts(hostMap, ep, "example.com")

		host := hostMap["www"][0]
		if host.TTL != "1800" {
			t.Errorf("TTL = %q, want %q (default)", host.TTL, "1800")
		}
	})

	t.Run("multiple A records same hostname", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep1 := &Endpoint{
			DNSName:    "www.example.com",
			RecordType: "A",
			RecordTTL:  300,
			Targets:    []string{"1.2.3.4"},
		}
		ep2 := &Endpoint{
			DNSName:    "www.example.com",
			RecordType: "A",
			RecordTTL:  300,
			Targets:    []string{"5.6.7.8"},
		}
		srv.applyEndpointToHosts(hostMap, ep1, "example.com")
		srv.applyEndpointToHosts(hostMap, ep2, "example.com")

		hosts := hostMap["www"]
		if len(hosts) != 2 {
			t.Fatalf("expected 2 hosts, got %d", len(hosts))
		}
		addrs := []string{hosts[0].Address, hosts[1].Address}
		if hosts[0].Address != "1.2.3.4" || hosts[1].Address != "5.6.7.8" {
			t.Errorf("Addresses = %v, want [1.2.3.4 5.6.7.8]", addrs)
		}
	})

	t.Run("multiple MX records same hostname", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep1 := &Endpoint{
			DNSName:    "example.com",
			RecordType: "MX",
			RecordTTL:  1800,
			Targets:    []string{"10 mail1.example.com"},
		}
		ep2 := &Endpoint{
			DNSName:    "example.com",
			RecordType: "MX",
			RecordTTL:  1800,
			Targets:    []string{"20 mail2.example.com"},
		}
		srv.applyEndpointToHosts(hostMap, ep1, "example.com")
		srv.applyEndpointToHosts(hostMap, ep2, "example.com")

		hosts := hostMap["@"]
		if len(hosts) != 2 {
			t.Fatalf("expected 2 hosts, got %d", len(hosts))
		}
		if hosts[0].MXPref != "10" || hosts[0].Address != "mail1.example.com" {
			t.Errorf("MX1 = (%q, %q), want (10, mail1.example.com)", hosts[0].MXPref, hosts[0].Address)
		}
		if hosts[1].MXPref != "20" || hosts[1].Address != "mail2.example.com" {
			t.Errorf("MX2 = (%q, %q), want (20, mail2.example.com)", hosts[1].MXPref, hosts[1].Address)
		}
	})

	t.Run("single endpoint with multiple A targets", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep := &Endpoint{
			DNSName:    "www.example.com",
			RecordType: "A",
			RecordTTL:  300,
			Targets:    []string{"1.2.3.4", "5.6.7.8", "9.10.11.12"},
		}
		srv.applyEndpointToHosts(hostMap, ep, "example.com")

		hosts := hostMap["www"]
		if len(hosts) != 3 {
			t.Fatalf("expected 3 hosts, got %d", len(hosts))
		}
		if hosts[0].Address != "1.2.3.4" || hosts[1].Address != "5.6.7.8" || hosts[2].Address != "9.10.11.12" {
			t.Errorf("Addresses = %v, want [1.2.3.4 5.6.7.8 9.10.11.12]", []string{hosts[0].Address, hosts[1].Address, hosts[2].Address})
		}
	})

	t.Run("single endpoint with multiple AAAA targets", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep := &Endpoint{
			DNSName:    "www.example.com",
			RecordType: "AAAA",
			RecordTTL:  300,
			Targets:    []string{"2001:db8::1", "2001:db8::2"},
		}
		srv.applyEndpointToHosts(hostMap, ep, "example.com")

		hosts := hostMap["www"]
		if len(hosts) != 2 {
			t.Fatalf("expected 2 hosts, got %d", len(hosts))
		}
		if hosts[0].Address != "2001:db8::1" || hosts[1].Address != "2001:db8::2" {
			t.Errorf("Addresses = %v, want [2001:db8::1 2001:db8::2]", []string{hosts[0].Address, hosts[1].Address})
		}
	})

	t.Run("single endpoint with multiple MX targets", func(t *testing.T) {
		hostMap := make(map[string][]*Host)
		ep := &Endpoint{
			DNSName:    "example.com",
			RecordType: "MX",
			RecordTTL:  1800,
			Targets:    []string{"10 mail1.example.com", "20 mail2.example.com"},
		}
		srv.applyEndpointToHosts(hostMap, ep, "example.com")

		hosts := hostMap["@"]
		if len(hosts) != 2 {
			t.Fatalf("expected 2 hosts, got %d", len(hosts))
		}
		if hosts[0].MXPref != "10" || hosts[0].Address != "mail1.example.com" {
			t.Errorf("MX1 = (%q, %q), want (10, mail1.example.com)", hosts[0].MXPref, hosts[0].Address)
		}
		if hosts[1].MXPref != "20" || hosts[1].Address != "mail2.example.com" {
			t.Errorf("MX2 = (%q, %q), want (20, mail2.example.com)", hosts[1].MXPref, hosts[1].Address)
		}
	})
}

func TestRemoveEndpointFromHosts(t *testing.T) {
	srv := &Server{}

	hostMap := make(map[string][]*Host)
	hostMap["www"] = []*Host{
		{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "300"},
		{Name: "www", Type: "AAAA", Address: "::1", TTL: "300"},
	}
	hostMap["@"] = []*Host{
		{Name: "@", Type: "A", Address: "5.6.7.8", TTL: "300"},
	}

	ep := &Endpoint{
		DNSName:    "www.example.com",
		RecordType: "A",
	}
	srv.removeEndpointFromHosts(hostMap, ep, "example.com")

	hosts := hostMap["www"]
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].Type != "AAAA" {
		t.Errorf("expected AAAA record to remain, got %s", hosts[0].Type)
	}

	if len(hostMap["@"]) != 1 {
		t.Errorf("expected @ to still have 1 record, got %d", len(hostMap["@"]))
	}
}

func TestRemoveEndpointFromHostsMultipleSameType(t *testing.T) {
	srv := &Server{}

	hostMap := make(map[string][]*Host)
	hostMap["www"] = []*Host{
		{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "300"},
		{Name: "www", Type: "A", Address: "5.6.7.8", TTL: "300"},
		{Name: "www", Type: "A", Address: "9.10.11.12", TTL: "300"},
	}

	ep := &Endpoint{
		DNSName:    "www.example.com",
		RecordType: "A",
	}
	srv.removeEndpointFromHosts(hostMap, ep, "example.com")

	hosts := hostMap["www"]
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts after removing one, got %d", len(hosts))
	}
}

func TestRemoveEndpointFromHostsWithTarget(t *testing.T) {
	srv := &Server{}

	hostMap := make(map[string][]*Host)
	hostMap["www"] = []*Host{
		{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "300"},
		{Name: "www", Type: "A", Address: "5.6.7.8", TTL: "300"},
		{Name: "www", Type: "A", Address: "9.10.11.12", TTL: "300"},
	}

	ep := &Endpoint{
		DNSName:    "www.example.com",
		RecordType: "A",
		Targets:    []string{"5.6.7.8"},
	}
	srv.removeEndpointFromHosts(hostMap, ep, "example.com")

	hosts := hostMap["www"]
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts after removing one, got %d", len(hosts))
	}
	if hosts[0].Address != "1.2.3.4" || hosts[1].Address != "9.10.11.12" {
		t.Errorf("expected [1.2.3.4 9.10.11.12], got %v", []string{hosts[0].Address, hosts[1].Address})
	}
}

func TestRemoveEndpointFromHostsNoMatch(t *testing.T) {
	srv := &Server{}

	hostMap := make(map[string][]*Host)
	hostMap["www"] = []*Host{
		{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "300"},
		{Name: "www", Type: "A", Address: "5.6.7.8", TTL: "300"},
	}

	ep := &Endpoint{
		DNSName:    "www.example.com",
		RecordType: "A",
		Targets:    []string{"9.9.9.9"},
	}
	srv.removeEndpointFromHosts(hostMap, ep, "example.com")

	hosts := hostMap["www"]
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts (no match), got %d", len(hosts))
	}
}

func TestMatchDomainFilter(t *testing.T) {
	tests := []struct {
		filter   string
		domain   string
		expected bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "www.example.com", true},
		{"example.com", "example.org", false},
		{".example.com", "example.com", true},
		{".example.com", "www.example.com", true},
		{".example.com", "example.org", false},
		{"", "anything.com", true},
		{"example.org", "example.com", false},
	}

	for _, tt := range tests {
		result := matchDomainFilter(tt.filter, tt.domain)
		if result != tt.expected {
			t.Errorf("matchDomainFilter(%q, %q) = %v, want %v",
				tt.filter, tt.domain, result, tt.expected)
		}
	}
}

func TestConfigDomainFilterMatch(t *testing.T) {
	tests := []struct {
		filters  []string
		domain   string
		expected bool
	}{
		{nil, "anything.com", true},
		{[]string{"example.com"}, "example.com", true},
		{[]string{"example.com"}, "www.example.com", true},
		{[]string{"example.com"}, "example.org", false},
		{[]string{"example.com", "example.org"}, "example.org", true},
	}

	for _, tt := range tests {
		cfg := &Config{DomainFilters: tt.filters}
		result := cfg.DomainFilterMatch(tt.domain)
		if result != tt.expected {
			t.Errorf("DomainFilterMatch(%q) with filters %v = %v, want %v",
				tt.domain, tt.filters, result, tt.expected)
		}
	}
}

func TestConfigGetDomainFilter(t *testing.T) {
	t.Run("empty filters", func(t *testing.T) {
		cfg := &Config{}
		df := cfg.GetDomainFilter()
		if len(df.Include) != 0 {
			t.Errorf("expected empty Include, got %v", df.Include)
		}
	})

	t.Run("with filters", func(t *testing.T) {
		cfg := &Config{DomainFilters: []string{"example.com", "example.org"}}
		df := cfg.GetDomainFilter()
		if len(df.Include) != 2 {
			t.Fatalf("expected 2 filters, got %d", len(df.Include))
		}
		if df.Include[0] != "example.com" {
			t.Errorf("expected first filter to be example.com, got %q", df.Include[0])
		}
		if df.Include[1] != "example.org" {
			t.Errorf("expected second filter to be example.org, got %q", df.Include[1])
		}
	})
}

func TestConfigValidate(t *testing.T) {
	t.Run("missing all", func(t *testing.T) {
		cfg := &Config{}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for missing config")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{
			APIUser:  "user",
			APIKey:   "key",
			Username: "user",
			ClientIP: "1.2.3.4",
		}
		err := cfg.Validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing api key", func(t *testing.T) {
		cfg := &Config{
			APIUser:  "user",
			Username: "user",
			ClientIP: "1.2.3.4",
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for missing API key")
		}
	})
}

func TestConfigAPIURL(t *testing.T) {
	t.Run("sandbox default", func(t *testing.T) {
		cfg := &Config{}
		if cfg.APIURL() != DefaultSandboxURL {
			t.Errorf("expected sandbox URL, got %q", cfg.APIURL())
		}
	})

	t.Run("production", func(t *testing.T) {
		cfg := &Config{Production: true}
		if cfg.APIURL() != DefaultProductionURL {
			t.Errorf("expected production URL, got %q", cfg.APIURL())
		}
	})
}

func TestParseTTL(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"1800", 1800},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
		{"3600", 3600},
	}

	for _, tt := range tests {
		result := ParseTTL(tt.input)
		if result != tt.expected {
			t.Errorf("ParseTTL(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestParseStringSlice(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"example.com,example.org", []string{"example.com", "example.org"}},
		{"example.com", []string{"example.com"}},
		{"", []string{}},
		{"  example.com  ,  example.org  ", []string{"example.com", "example.org"}},
		{"example.com,,example.org", []string{"example.com", "example.org"}},
	}

	for _, tt := range tests {
		result := parseStringSlice(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseStringSlice(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("parseStringSlice(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
			}
		}
	}
}

func TestDomainFilterJSON(t *testing.T) {
	df := DomainFilter{Include: []string{"example.com", "example.org"}}
	data, err := MarshalDomainFilter(df)
	if err != nil {
		t.Fatalf("MarshalDomainFilter failed: %v", err)
	}

	var result DomainFilter
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(result.Include) != 2 {
		t.Errorf("expected 2 includes, got %d", len(result.Include))
	}
}

func TestChangesJSON(t *testing.T) {
	changes := &Changes{
		Create: []*Endpoint{
			{
				DNSName:    "test.example.com",
				RecordType: "A",
				RecordTTL:  300,
				Targets:    []string{"1.2.3.4"},
			},
		},
		Delete: []*Endpoint{
			{
				DNSName:    "old.example.com",
				RecordType: "A",
				RecordTTL:  300,
				Targets:    []string{"5.6.7.8"},
			},
		},
	}

	data, err := json.Marshal(changes)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result Changes
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(result.Create) != 1 {
		t.Errorf("expected 1 create, got %d", len(result.Create))
	}
	if result.Create[0].DNSName != "test.example.com" {
		t.Errorf("expected DNSName test.example.com, got %q", result.Create[0].DNSName)
	}
	if len(result.Delete) != 1 {
		t.Errorf("expected 1 delete, got %d", len(result.Delete))
	}
}

func TestEnvOrFlag(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		t.Setenv("TEST_KEY", "test_value")
		var target string
		envOrFlag("TEST_KEY", &target)
		if target != "test_value" {
			t.Errorf("expected test_value, got %q", target)
		}
	})

	t.Run("bool true", func(t *testing.T) {
		t.Setenv("TEST_BOOL", "true")
		var target bool
		envOrFlag("TEST_BOOL", &target)
		if !target {
			t.Error("expected true")
		}
	})

	t.Run("bool false", func(t *testing.T) {
		t.Setenv("TEST_BOOL", "false")
		var target bool
		envOrFlag("TEST_BOOL", &target)
		if target {
			t.Error("expected false")
		}
	})

	t.Run("duration", func(t *testing.T) {
		t.Setenv("TEST_DURATION", "30s")
		var target time.Duration
		envOrFlag("TEST_DURATION", &target)
		if target != 30*time.Second {
			t.Errorf("expected 30s, got %v", target)
		}
	})

	t.Run("string slice", func(t *testing.T) {
		t.Setenv("TEST_SLICE", "a.com,b.com,c.com")
		var target stringSliceFlag
		envOrFlag("TEST_SLICE", &target)
		if len(target) != 3 {
			t.Fatalf("expected 3 items, got %d", len(target))
		}
		if target[0] != "a.com" || target[1] != "b.com" || target[2] != "c.com" {
			t.Errorf("expected [a.com b.com c.com], got %v", target)
		}
	})

	t.Run("empty env", func(t *testing.T) {
		t.Setenv("TEST_EMPTY", "")
		var target string = "original"
		envOrFlag("TEST_EMPTY", &target)
		if target != "original" {
			t.Errorf("expected original, got %q", target)
		}
	})
}

type mockClient struct {
	domains []Domain
	hosts   map[string][]Host
}

func (m *mockClient) Init() error                                              { return nil }
func (m *mockClient) GetDomains() ([]Domain, error)                            { return m.domains, nil }
func (m *mockClient) GetHosts(sld, tld string) ([]Host, error)                 { key := sld + "." + tld; return m.hosts[key], nil }
func (m *mockClient) SetHosts(sld, tld string, hosts []Host) error { key := sld + "." + tld; m.hosts[key] = hosts; return nil }
func (m *mockClient) SplitDomain(domain string) (string, string, string)       { return "example", "com", "@" }
func (m *mockClient) TLDs() []string                                           { return []string{"com"} }
func (m *mockClient) TLDCount() int                                            { return 1 }

func TestGetRecords_MergesMultipleTargets(t *testing.T) {
	mock := &mockClient{
		domains: []Domain{{Name: "example.com"}},
		hosts: map[string][]Host{
			"example.com": {
				{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "1800"},
				{Name: "www", Type: "A", Address: "5.6.7.8", TTL: "1800"},
				{Name: "www", Type: "AAAA", Address: "2001:db8::1", TTL: "1800"},
				{Name: "www", Type: "AAAA", Address: "2001:db8::2", TTL: "1800"},
				{Name: "@", Type: "A", Address: "9.10.11.12", TTL: "1800"},
			},
		},
	}
	srv := &Server{client: mock, cfg: &Config{}}

	endpoints, err := srv.getRecords(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(endpoints))
	}

	for _, ep := range endpoints {
		if ep.DNSName == "www.example.com" && ep.RecordType == "A" {
			if len(ep.Targets) != 2 {
				t.Errorf("www A: expected 2 targets, got %d: %v", len(ep.Targets), ep.Targets)
			}
			if ep.Targets[0] != "1.2.3.4" || ep.Targets[1] != "5.6.7.8" {
				t.Errorf("www A: targets = %v, want [1.2.3.4 5.6.7.8]", ep.Targets)
			}
		}
		if ep.DNSName == "www.example.com" && ep.RecordType == "AAAA" {
			if len(ep.Targets) != 2 {
				t.Errorf("www AAAA: expected 2 targets, got %d: %v", len(ep.Targets), ep.Targets)
			}
			if ep.Targets[0] != "2001:db8::1" || ep.Targets[1] != "2001:db8::2" {
				t.Errorf("www AAAA: targets = %v, want [2001:db8::1 2001:db8::2]", ep.Targets)
			}
		}
		if ep.DNSName == "example.com" && ep.RecordType == "A" {
			if len(ep.Targets) != 1 {
				t.Errorf("apex A: expected 1 target, got %d: %v", len(ep.Targets), ep.Targets)
			}
		}
	}
}

func TestApplyChangesForDomain_UpdatePartialTargetRemoval(t *testing.T) {
	mock := &mockClient{
		domains: []Domain{{Name: "example.com"}},
		hosts: map[string][]Host{
			"example.com": {
				{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "1800"},
				{Name: "www", Type: "A", Address: "5.6.7.8", TTL: "1800"},
			},
		},
	}
	srv := &Server{client: mock, cfg: &Config{}}

	dc := &domainChanges{
		updateOld: []*Endpoint{
			{DNSName: "www.example.com", RecordType: "A", Targets: []string{"1.2.3.4", "5.6.7.8"}},
		},
		updateNew: []*Endpoint{
			{DNSName: "www.example.com", RecordType: "A", Targets: []string{"1.2.3.4"}},
		},
	}

	err := srv.applyChangesForDomain(context.Background(), "example.com", dc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hosts := mock.hosts["example.com"]
	var wwwHosts []Host
	for _, h := range hosts {
		if h.Name == "www" && h.Type == "A" {
			wwwHosts = append(wwwHosts, h)
		}
	}
	if len(wwwHosts) != 1 {
		t.Fatalf("expected 1 www A host after update, got %d", len(wwwHosts))
	}
	if wwwHosts[0].Address != "1.2.3.4" {
		t.Errorf("expected www A address 1.2.3.4, got %s", wwwHosts[0].Address)
	}
}

func TestApplyChangesForDomain_UpdateAddTarget(t *testing.T) {
	mock := &mockClient{
		domains: []Domain{{Name: "example.com"}},
		hosts: map[string][]Host{
			"example.com": {
				{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "1800"},
			},
		},
	}
	srv := &Server{client: mock, cfg: &Config{}}

	dc := &domainChanges{
		updateOld: []*Endpoint{
			{DNSName: "www.example.com", RecordType: "A", Targets: []string{"1.2.3.4"}},
		},
		updateNew: []*Endpoint{
			{DNSName: "www.example.com", RecordType: "A", Targets: []string{"1.2.3.4", "5.6.7.8"}},
		},
	}

	err := srv.applyChangesForDomain(context.Background(), "example.com", dc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hosts := mock.hosts["example.com"]
	var wwwHosts []Host
	for _, h := range hosts {
		if h.Name == "www" && h.Type == "A" {
			wwwHosts = append(wwwHosts, h)
		}
	}
	if len(wwwHosts) != 2 {
		t.Fatalf("expected 2 www A hosts after update, got %d", len(wwwHosts))
	}
}

func TestApplyChangesForDomain_DeleteAllTargets(t *testing.T) {
	mock := &mockClient{
		domains: []Domain{{Name: "example.com"}},
		hosts: map[string][]Host{
			"example.com": {
				{Name: "www", Type: "A", Address: "1.2.3.4", TTL: "1800"},
				{Name: "www", Type: "A", Address: "5.6.7.8", TTL: "1800"},
			},
		},
	}
	srv := &Server{client: mock, cfg: &Config{}}

	dc := &domainChanges{
		delete: []*Endpoint{
			{DNSName: "www.example.com", RecordType: "A", Targets: []string{"1.2.3.4", "5.6.7.8"}},
		},
	}

	err := srv.applyChangesForDomain(context.Background(), "example.com", dc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hosts := mock.hosts["example.com"]
	var wwwHosts []Host
	for _, h := range hosts {
		if h.Name == "www" && h.Type == "A" {
			wwwHosts = append(wwwHosts, h)
		}
	}
	if len(wwwHosts) != 0 {
		t.Fatalf("expected 0 www A hosts after delete, got %d", len(wwwHosts))
	}
}
