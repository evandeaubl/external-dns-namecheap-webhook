package webhook

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, server *httptest.Server) *NamecheapClient {
	t.Helper()
	return &NamecheapClient{
		httpClient:      &http.Client{Timeout: 5 * time.Second},
		baseURL:         server.URL,
		apiUser:         "testuser",
		apiKey:          "testkey",
		username:        "testuser",
		clientIP:        "1.2.3.4",
		tlds:            []string{"com", "co.uk", "net", "org"},
		tldSet: map[string]bool{
			"com":   true,
			"co.uk": true,
			"net":   true,
			"org":   true,
		},
		hostsCache: make(map[string]cachedHosts),
	}
}

func TestNamecheapClient_FetchTLDList(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.getTldList">
    <Tlds>
      <Tld Name="com" />
      <Tld Name="co.uk" />
      <Tld Name="net" />
      <Tld Name="org" />
    </Tlds>
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, xmlResponse)
	}))
	defer server.Close()

	client := &NamecheapClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    server.URL,
		apiUser:    "testuser",
		apiKey:     "testkey",
		username:   "testuser",
		clientIP:   "1.2.3.4",
	}

	if err := client.fetchTLDList(); err != nil {
		t.Fatalf("fetchTLDList failed: %v", err)
	}

	if len(client.tlds) != 4 {
		t.Fatalf("expected 4 TLDs, got %d", len(client.tlds))
	}

	for _, expected := range []string{"com", "co.uk", "net", "org"} {
		if !client.tldSet[expected] {
			t.Errorf("expected TLD %q to be in set", expected)
		}
	}

	if len(client.tlds) > 1 {
		if len(client.tlds[0]) < len(client.tlds[1]) {
			t.Error("expected TLDs to be sorted by length descending")
		}
	}
}

func TestNamecheapClient_GetDomains(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.getList">
    <DomainGetListResult>
      <Domain ID="1" Name="example.com" User="testuser" Created="01/01/2020" Expires="01/01/2025" IsExpired="false" IsLocked="false" AutoRenew="false" WhoisGuard="ENABLED" IsPremium="false" IsOurDNS="true"/>
      <Domain ID="2" Name="example.co.uk" User="testuser" Created="01/01/2020" Expires="01/01/2025" IsExpired="false" IsLocked="false" AutoRenew="false" WhoisGuard="NOTPRESENT" IsPremium="false" IsOurDNS="true"/>
    </DomainGetListResult>
    <Paging>
      <TotalItems>2</TotalItems>
      <CurrentPage>1</CurrentPage>
      <PageSize>100</PageSize>
    </Paging>
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, xmlResponse)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	domains, err := client.GetDomains()
	if err != nil {
		t.Fatalf("GetDomains failed: %v", err)
	}

	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}

	if domains[0].Name != "example.com" {
		t.Errorf("expected first domain to be example.com, got %q", domains[0].Name)
	}
	if domains[1].Name != "example.co.uk" {
		t.Errorf("expected second domain to be example.co.uk, got %q", domains[1].Name)
	}
}

func TestNamecheapClient_GetHosts(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.dns.getHosts">
    <DomainDNSGetHostsResult Domain="example.com" IsUsingOurDNS="true">
      <host HostId="1" Name="@" Type="A" Address="1.2.3.4" MXPref="10" TTL="1800" />
      <host HostId="2" Name="www" Type="A" Address="122.23.3.7" MXPref="10" TTL="1800" />
      <host HostId="3" Name="@" Type="MX" Address="mail.example.com" MXPref="10" TTL="3600" />
    </DomainDNSGetHostsResult>
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, xmlResponse)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	hosts, err := client.GetHosts("example", "com")
	if err != nil {
		t.Fatalf("GetHosts failed: %v", err)
	}

	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}

	if hosts[0].Name != "@" || hosts[0].Type != "A" || hosts[0].Address != "1.2.3.4" {
		t.Errorf("unexpected host[0]: %+v", hosts[0])
	}
	if hosts[1].Name != "www" || hosts[1].Type != "A" || hosts[1].Address != "122.23.3.7" {
		t.Errorf("unexpected host[1]: %+v", hosts[1])
	}
	if hosts[2].Name != "@" || hosts[2].Type != "MX" || hosts[2].MXPref != "10" {
		t.Errorf("unexpected host[2]: %+v", hosts[2])
	}
}

func TestNamecheapClient_SetHosts(t *testing.T) {
	var receivedParams url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedParams = r.URL.Query()
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.dns.setHosts">
    <DomainDNSSetHostsResult Domain="example.com" IsSuccess="true" />
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	hosts := []Host{
		{Name: "@", Type: "A", Address: "1.2.3.4", TTL: "1800"},
		{Name: "www", Type: "A", Address: "5.6.7.8", TTL: "300"},
		{Name: "@", Type: "MX", Address: "mail.example.com", MXPref: "10", TTL: "3600"},
	}

	if err := client.SetHosts("example", "com", hosts); err != nil {
		t.Fatalf("SetHosts failed: %v", err)
	}

	if receivedParams.Get("SLD") != "example" {
		t.Errorf("expected SLD=example, got %q", receivedParams.Get("SLD"))
	}
	if receivedParams.Get("TLD") != "com" {
		t.Errorf("expected TLD=com, got %q", receivedParams.Get("TLD"))
	}
	if receivedParams.Get("HostName1") != "@" {
		t.Errorf("expected HostName1=@, got %q", receivedParams.Get("HostName1"))
	}
	if receivedParams.Get("RecordType1") != "A" {
		t.Errorf("expected RecordType1=A, got %q", receivedParams.Get("RecordType1"))
	}
	if receivedParams.Get("Address1") != "1.2.3.4" {
		t.Errorf("expected Address1=1.2.3.4, got %q", receivedParams.Get("Address1"))
	}
	if receivedParams.Get("HostName2") != "www" {
		t.Errorf("expected HostName2=www, got %q", receivedParams.Get("HostName2"))
	}
	if receivedParams.Get("MXPref3") != "10" {
		t.Errorf("expected MXPref3=10, got %q", receivedParams.Get("MXPref3"))
	}
}

func TestNamecheapClient_APIError(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="ERROR" xmlns="http://api.namecheap.com/xml.response">
  <Errors>
    <Error Number="1011102">API Key is invalid or API access has not been enabled</Error>
  </Errors>
  <CommandResponse />
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, xmlResponse)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	_, err := client.GetDomains()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "API Key is invalid") {
		t.Errorf("expected error about API key, got: %v", err)
	}
}

func TestNamecheapClient_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	client := newTestClient(t, server)

	_, err := client.GetDomains()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 error, got: %v", err)
	}
}

func TestNamecheapClient_SplitDomainWithEmptyTLDList(t *testing.T) {
	client := &NamecheapClient{
		tlds:   []string{},
		tldSet: map[string]bool{},
	}

	sld, tld, host := client.SplitDomain("www.example.com")
	if sld != "example" || tld != "com" || host != "www" {
		t.Errorf("SplitDomain(www.example.com) = (%q, %q, %q), want (example, com, www)", sld, tld, host)
	}

	sld, tld, host = client.SplitDomain("example.com")
	if sld != "example" || tld != "com" || host != "@" {
		t.Errorf("SplitDomain(example.com) = (%q, %q, %q), want (example, com, @)", sld, tld, host)
	}
}

func TestXMLParsing_TldList(t *testing.T) {
	xmlData := `<Tlds>
      <Tld Name="com" />
      <Tld Name="co.uk" />
      <Tld Name="net" />
    </Tlds>`

	var tldList TldList
	wrapped := "<CommandResponse>" + xmlData + "</CommandResponse>"
	if err := xml.Unmarshal([]byte(wrapped), &tldList); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(tldList.Tlds.Tld) != 3 {
		t.Fatalf("expected 3 TLDs, got %d", len(tldList.Tlds.Tld))
	}
	if tldList.Tlds.Tld[0].Name != "com" {
		t.Errorf("expected first TLD to be com, got %q", tldList.Tlds.Tld[0].Name)
	}
}

func TestXMLParsing_HostsResult(t *testing.T) {
	xmlData := `<DomainDNSGetHostsResult Domain="example.com" IsUsingOurDNS="true">
      <host HostId="1" Name="@" Type="A" Address="1.2.3.4" MXPref="10" TTL="1800" />
      <host HostId="2" Name="www" Type="CNAME" Address="target.example.org" TTL="3600" />
    </DomainDNSGetHostsResult>`

	var hostsResult HostsResult
	wrapped := "<CommandResponse>" + xmlData + "</CommandResponse>"
	if err := xml.Unmarshal([]byte(wrapped), &hostsResult); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if hostsResult.Result.Domain != "example.com" {
		t.Errorf("expected Domain=example.com, got %q", hostsResult.Result.Domain)
	}
	if len(hostsResult.Result.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hostsResult.Result.Hosts))
	}
	if hostsResult.Result.Hosts[0].Name != "@" || hostsResult.Result.Hosts[0].Type != "A" {
		t.Errorf("unexpected host[0]: %+v", hostsResult.Result.Hosts[0])
	}
	if hostsResult.Result.Hosts[1].Name != "www" || hostsResult.Result.Hosts[1].Type != "CNAME" {
		t.Errorf("unexpected host[1]: %+v", hostsResult.Result.Hosts[1])
	}
}

func TestXMLParsing_SetHostsResult(t *testing.T) {
	xmlData := `<DomainDNSSetHostsResult Domain="example.com" IsSuccess="true" />`

	var setHostsResult SetHostsResult
	wrapped := "<CommandResponse>" + xmlData + "</CommandResponse>"
	if err := xml.Unmarshal([]byte(wrapped), &setHostsResult); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if setHostsResult.Result.Domain != "example.com" {
		t.Errorf("expected Domain=example.com, got %q", setHostsResult.Result.Domain)
	}
	if setHostsResult.Result.IsSuccess != "true" {
		t.Errorf("expected IsSuccess=true, got %q", setHostsResult.Result.IsSuccess)
	}
}

func TestNewNamecheapClient_DefaultUsername(t *testing.T) {
	cfg := &Config{
		APIUser:  "myapiuser",
		APIKey:   "mykey",
		Username: "",
	}

	client := NewNamecheapClient(cfg)
	if client.username != "myapiuser" {
		t.Errorf("expected username to default to APIUser 'myapiuser', got %q", client.username)
	}
}

func TestNewNamecheapClient_ExplicitUsername(t *testing.T) {
	cfg := &Config{
		APIUser:  "myapiuser",
		APIKey:   "mykey",
		Username: "otheruser",
	}

	client := NewNamecheapClient(cfg)
	if client.username != "otheruser" {
		t.Errorf("expected username to be 'otheruser', got %q", client.username)
	}
}

func TestNewNamecheapClient_DefaultClientIP(t *testing.T) {
	cfg := &Config{
		APIUser:  "myapiuser",
		APIKey:   "mykey",
		ClientIP: "",
	}

	client := NewNamecheapClient(cfg)
	if client.clientIP != "127.0.0.1" {
		t.Errorf("expected clientIP to default to 127.0.0.1, got %q", client.clientIP)
	}
}

func TestNewNamecheapClient_ExplicitClientIP(t *testing.T) {
	cfg := &Config{
		APIUser:  "myapiuser",
		APIKey:   "mykey",
		ClientIP: "5.6.7.8",
	}

	client := NewNamecheapClient(cfg)
	if client.clientIP != "5.6.7.8" {
		t.Errorf("expected clientIP to be 5.6.7.8, got %q", client.clientIP)
	}
}

func TestXMLParsing_NcErrors(t *testing.T) {
	xmlData := `<Errors>
    <Error Number="1010101">Parameter APIUser is missing</Error>
    <Error Number="1011102">Parameter APIKey is missing</Error>
  </Errors>`

	var errors NcErrors
	if err := xml.Unmarshal([]byte(xmlData), &errors); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(errors.ErrorList) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors.ErrorList))
	}
	if errors.ErrorList[0].Number != "1010101" {
		t.Errorf("expected first error number 1010101, got %q", errors.ErrorList[0].Number)
	}
	if errors.ErrorList[0].Message != "Parameter APIUser is missing" {
		t.Errorf("expected first error message, got %q", errors.ErrorList[0].Message)
	}

	errMsg := errors.Error()
	if !strings.Contains(errMsg, "1010101") || !strings.Contains(errMsg, "1011102") {
		t.Errorf("expected error message to contain both error numbers, got: %s", errMsg)
	}
}

func TestNamecheapClient_GetDomainsCaching(t *testing.T) {
	xmlDomainsResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.getList">
    <DomainGetListResult>
      <Domain ID="1" Name="example.com" User="testuser" Created="01/01/2020" Expires="01/01/2025" IsExpired="false" IsLocked="false" AutoRenew="false" WhoisGuard="ENABLED" IsPremium="false" IsOurDNS="true"/>
    </DomainGetListResult>
    <Paging>
      <TotalItems>1</TotalItems>
      <CurrentPage>1</CurrentPage>
      <PageSize>100</PageSize>
    </Paging>
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, xmlDomainsResponse)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	client.domainCacheTTL = 5 * time.Minute

	domains1, err := client.GetDomains()
	if err != nil {
		t.Fatalf("first GetDomains failed: %v", err)
	}
	if len(domains1) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(domains1))
	}
	if requestCount != 1 {
		t.Fatalf("expected 1 API request, got %d", requestCount)
	}

	domains2, err := client.GetDomains()
	if err != nil {
		t.Fatalf("second GetDomains failed: %v", err)
	}
	if len(domains2) != 1 {
		t.Fatalf("expected 1 domain on second call, got %d", len(domains2))
	}
	if requestCount != 1 {
		t.Fatalf("expected cache hit (still 1 request), got %d", requestCount)
	}
}

func TestNamecheapClient_GetHostsCaching(t *testing.T) {
	xmlHostsResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.dns.getHosts">
    <DomainDNSGetHostsResult Domain="example.com" IsUsingOurDNS="true">
      <host HostId="1" Name="@" Type="A" Address="1.2.3.4" MXPref="10" TTL="1800" />
      <host HostId="2" Name="www" Type="A" Address="5.6.7.8" MXPref="10" TTL="1800" />
    </DomainDNSGetHostsResult>
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, xmlHostsResponse)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	client.hostsCacheTTL = 5 * time.Minute

	hosts1, err := client.GetHosts("example", "com")
	if err != nil {
		t.Fatalf("first GetHosts failed: %v", err)
	}
	if len(hosts1) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts1))
	}
	if requestCount != 1 {
		t.Fatalf("expected 1 API request, got %d", requestCount)
	}

	hosts2, err := client.GetHosts("example", "com")
	if err != nil {
		t.Fatalf("second GetHosts failed: %v", err)
	}
	if len(hosts2) != 2 {
		t.Fatalf("expected 2 hosts on second call, got %d", len(hosts2))
	}
	if requestCount != 1 {
		t.Fatalf("expected cache hit (still 1 request), got %d", requestCount)
	}

	hosts3, err := client.GetHosts("other", "net")
	if err != nil {
		t.Fatalf("GetHosts for other domain failed: %v", err)
	}
	if len(hosts3) != 2 {
		t.Fatalf("expected 2 hosts for other domain (same XML response), got %d", len(hosts3))
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 requests (different domain not cached), got %d", requestCount)
	}
}

func TestNamecheapClient_CacheInvalidationOnSetHosts(t *testing.T) {
	xmlHostsResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.dns.getHosts">
    <DomainDNSGetHostsResult Domain="example.com" IsUsingOurDNS="true">
      <host HostId="1" Name="@" Type="A" Address="1.2.3.4" MXPref="10" TTL="1800" />
    </DomainDNSGetHostsResult>
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	xmlSetHostsResponse := `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse Status="OK" xmlns="http://api.namecheap.com/xml.response">
  <Errors />
  <CommandResponse Type="namecheap.domains.dns.setHosts">
    <DomainDNSSetHostsResult Domain="example.com" IsSuccess="true" />
  </CommandResponse>
  <Server>TestServer</Server>
  <GMTTimeDifference>+0</GMTTimeDifference>
  <ExecutionTime>0.1</ExecutionTime>
</ApiResponse>`

	requestCount := 0
	var lastCommand string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		lastCommand = r.URL.Query().Get("Command")
		w.Header().Set("Content-Type", "text/xml")
		if lastCommand == "namecheap.domains.dns.setHosts" {
			_, _ = fmt.Fprint(w, xmlSetHostsResponse)
		} else {
			_, _ = fmt.Fprint(w, xmlHostsResponse)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	client.hostsCacheTTL = 5 * time.Minute

	_, err := client.GetHosts("example", "com")
	if err != nil {
		t.Fatalf("GetHosts failed: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected 1 request, got %d", requestCount)
	}

	_, err = client.GetHosts("example", "com")
	if err != nil {
		t.Fatalf("second GetHosts failed: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected cache hit (still 1 request), got %d", requestCount)
	}

	err = client.SetHosts("example", "com", []Host{
		{Name: "@", Type: "A", Address: "9.9.9.9", TTL: "1800"},
	})
	if err != nil {
		t.Fatalf("SetHosts failed: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 requests after SetHosts, got %d", requestCount)
	}

	_, err = client.GetHosts("example", "com")
	if err != nil {
		t.Fatalf("third GetHosts failed: %v", err)
	}
	if requestCount != 3 {
		t.Fatalf("expected 3 requests after cache invalidation, got %d", requestCount)
	}
}
