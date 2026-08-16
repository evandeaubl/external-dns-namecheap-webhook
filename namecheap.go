package webhook

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type NamecheapClient struct {
	httpClient *http.Client
	baseURL    string
	apiUser    string
	apiKey     string
	username   string
	clientIP   string
	tlds       []string
	tldSet     map[string]bool
}

type DNSProvider interface {
	GetDomains() ([]Domain, error)
	GetHosts(sld, tld string) ([]Host, error)
	SetHosts(sld, tld string, hosts []Host) error
	SplitDomain(domain string) (string, string, string)
	TLDs() []string
	TLDCount() int
}

type ncError struct {
	Number  string `xml:"Number,attr"`
	Message string `xml:",chardata"`
}

type ApiResponse struct {
	XMLName         xml.Name         `xml:"ApiResponse"`
	Status          string           `xml:"Status,attr"`
	Errors          *NcErrors        `xml:"Errors"`
	CommandResponse CommandResponse  `xml:"CommandResponse"`
	Server          string           `xml:"Server"`
	GMTTime         string           `xml:"GMTTimeDifference"`
	ExecTime        string           `xml:"ExecutionTime"`
}

type CommandResponse struct {
	XMLName  xml.Name `xml:"CommandResponse"`
	Type     string   `xml:"Type,attr"`
	InnerXML string   `xml:",innerxml"`
}

type NcErrors struct {
	ErrorList []ncError `xml:"Error"`
}

func (e *NcErrors) Error() string {
	if e == nil || len(e.ErrorList) == 0 {
		return ""
	}
	msgs := make([]string, len(e.ErrorList))
	for i, err := range e.ErrorList {
		msgs[i] = fmt.Sprintf("[%s] %s", err.Number, err.Message)
	}
	return strings.Join(msgs, "; ")
}

type DomainsList struct {
	XMLName xml.Name          `xml:"CommandResponse"`
	Result  DomainsListResult `xml:"DomainGetListResult"`
	Paging  Paging            `xml:"Paging"`
}

type DomainsListResult struct {
	Domains []Domain `xml:"Domain"`
}

type Domain struct {
	ID         string `xml:"ID,attr"`
	Name       string `xml:"Name,attr"`
	User       string `xml:"User,attr"`
	Created    string `xml:"Created,attr"`
	Expires    string `xml:"Expires,attr"`
	IsExpired  string `xml:"IsExpired,attr"`
	IsLocked   string `xml:"IsLocked,attr"`
	AutoRenew  string `xml:"AutoRenew,attr"`
	WhoisGuard string `xml:"WhoisGuard,attr"`
	IsPremium  string `xml:"IsPremium,attr"`
	IsOurDNS   string `xml:"IsOurDNS,attr"`
}

type Paging struct {
	TotalItems  int `xml:"TotalItems"`
	CurrentPage int `xml:"CurrentPage"`
	PageSize    int `xml:"PageSize"`
}

type TldList struct {
	XMLName xml.Name `xml:"CommandResponse"`
	Tlds    Tlds     `xml:"Tlds"`
}

type Tlds struct {
	Tld []Tld `xml:"Tld"`
}

type Tld struct {
	Name string `xml:"Name,attr"`
}

type HostsResult struct {
	XMLName xml.Name         `xml:"CommandResponse"`
	Result  HostsResultInner `xml:"DomainDNSGetHostsResult"`
}

type HostsResultInner struct {
	Domain        string `xml:"Domain,attr"`
	IsUsingOurDNS string `xml:"IsUsingOurDNS,attr"`
	Hosts         []Host `xml:"host"`
}

type Host struct {
	HostId  string `xml:"HostId,attr"`
	Name    string `xml:"Name,attr"`
	Type    string `xml:"Type,attr"`
	Address string `xml:"Address,attr"`
	MXPref  string `xml:"MXPref,attr"`
	TTL     string `xml:"TTL,attr"`
}

type SetHostsResult struct {
	XMLName xml.Name            `xml:"CommandResponse"`
	Result  SetHostsResultInner `xml:"DomainDNSSetHostsResult"`
}

type SetHostsResultInner struct {
	Domain    string `xml:"Domain,attr"`
	IsSuccess string `xml:"IsSuccess,attr"`
}

func NewNamecheapClient(cfg *Config) *NamecheapClient {
	return &NamecheapClient{
		httpClient: &http.Client{
			Timeout: cfg.RequestTTL,
		},
		baseURL:  cfg.APIURL(),
		apiUser:  cfg.APIUser,
		apiKey:   cfg.APIKey,
		username: cfg.Username,
		clientIP: cfg.ClientIP,
	}
}

func (c *NamecheapClient) Init() error {
	return c.fetchTLDList()
}

func (c *NamecheapClient) fetchTLDList() error {
	resp, err := c.makeRequest("namecheap.domains.getTldList", nil)
	if err != nil {
		return fmt.Errorf("failed to fetch TLD list: %w", err)
	}

	var tldList TldList
	if err := c.parseCommandResponse(resp, &tldList); err != nil {
		return fmt.Errorf("failed to parse TLD list: %w", err)
	}

	c.tlds = make([]string, 0, len(tldList.Tlds.Tld))
	c.tldSet = make(map[string]bool)
	for _, tld := range tldList.Tlds.Tld {
		name := strings.ToLower(strings.TrimSpace(tld.Name))
		if name != "" && !c.tldSet[name] {
			c.tlds = append(c.tlds, name)
			c.tldSet[name] = true
		}
	}

	sort.Slice(c.tlds, func(i, j int) bool {
		return len(c.tlds[i]) > len(c.tlds[j])
	})

	return nil
}

func (c *NamecheapClient) GetDomains() ([]Domain, error) {
	params := url.Values{}
	params.Set("Page", "1")
	params.Set("PageSize", "100")
	params.Set("SortBy", "NAME")

	resp, err := c.makeRequest("namecheap.domains.getList", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get domains: %w", err)
	}

	var domainsList DomainsList
	if err := c.parseCommandResponse(resp, &domainsList); err != nil {
		return nil, fmt.Errorf("failed to parse domains list: %w", err)
	}

	return domainsList.Result.Domains, nil
}

func (c *NamecheapClient) GetHosts(sld, tld string) ([]Host, error) {
	params := url.Values{}
	params.Set("SLD", sld)
	params.Set("TLD", tld)

	resp, err := c.makeRequest("namecheap.domains.dns.getHosts", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosts for %s.%s: %w", sld, tld, err)
	}

	var hostsResult HostsResult
	if err := c.parseCommandResponse(resp, &hostsResult); err != nil {
		return nil, fmt.Errorf("failed to parse hosts for %s.%s: %w", sld, tld, err)
	}

	return hostsResult.Result.Hosts, nil
}

func (c *NamecheapClient) SetHosts(sld, tld string, hosts []Host) error {
	params := url.Values{}
	params.Set("SLD", sld)
	params.Set("TLD", tld)

	for i, host := range hosts {
		idx := i + 1
		params.Set(fmt.Sprintf("HostName%d", idx), host.Name)
		params.Set(fmt.Sprintf("RecordType%d", idx), host.Type)
		params.Set(fmt.Sprintf("Address%d", idx), host.Address)
		if host.MXPref != "" {
			params.Set(fmt.Sprintf("MXPref%d", idx), host.MXPref)
		}
		if host.TTL != "" {
			params.Set(fmt.Sprintf("TTL%d", idx), host.TTL)
		}
	}

	resp, err := c.makeRequest("namecheap.domains.dns.setHosts", params)
	if err != nil {
		return fmt.Errorf("failed to set hosts for %s.%s: %w", sld, tld, err)
	}

	var setHostsResult SetHostsResult
	if err := c.parseCommandResponse(resp, &setHostsResult); err != nil {
		return fmt.Errorf("failed to parse set hosts response for %s.%s: %w", sld, tld, err)
	}

	if setHostsResult.Result.IsSuccess != "true" {
		return fmt.Errorf("set hosts for %s.%s was not successful", sld, tld)
	}

	return nil
}

func (c *NamecheapClient) makeRequest(command string, params url.Values) (*ApiResponse, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("ApiUser", c.apiUser)
	params.Set("ApiKey", c.apiKey)
	params.Set("UserName", c.username)
	params.Set("ClientIp", c.clientIP)
	params.Set("Command", command)

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp ApiResponse
	if err := xml.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal XML: %w", err)
	}

	if apiResp.Status != "OK" {
		if apiResp.Errors != nil && len(apiResp.Errors.ErrorList) > 0 {
			return nil, fmt.Errorf("API error: %s", apiResp.Errors.Error())
		}
		return nil, fmt.Errorf("API returned non-OK status")
	}

	return &apiResp, nil
}

func (c *NamecheapClient) parseCommandResponse(resp *ApiResponse, target interface{}) error {
	if resp.CommandResponse.InnerXML == "" {
		return fmt.Errorf("no CommandResponse in API response")
	}

	wrapped := "<CommandResponse>" + resp.CommandResponse.InnerXML + "</CommandResponse>"
	if err := xml.Unmarshal([]byte(wrapped), target); err != nil {
		return fmt.Errorf("failed to unmarshal CommandResponse: %w", err)
	}

	return nil
}

func (c *NamecheapClient) SplitDomain(domain string) (sld, tld, hostname string) {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if domain == "" {
		return "", "", ""
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", "", ""
	}

	for _, knownTLD := range c.tlds {
		tldLabels := strings.Split(knownTLD, ".")
		if len(labels) < len(tldLabels) {
			continue
		}

		domainSuffix := strings.Join(labels[len(labels)-len(tldLabels):], ".")
		if domainSuffix == knownTLD {
			tld = knownTLD
			sld = labels[len(labels)-len(tldLabels)-1]
			hostnameLabels := labels[:len(labels)-len(tldLabels)-1]
			if len(hostnameLabels) == 0 {
				hostname = "@"
			} else {
				hostname = strings.Join(hostnameLabels, ".")
			}
			return sld, tld, hostname
		}
	}

	tld = labels[len(labels)-1]
	sld = labels[len(labels)-2]
	hostnameLabels := labels[:len(labels)-2]
	if len(hostnameLabels) == 0 {
		hostname = "@"
	} else {
		hostname = strings.Join(hostnameLabels, ".")
	}
	return sld, tld, hostname
}

func (c *NamecheapClient) TLDCount() int {
	return len(c.tlds)
}

func (c *NamecheapClient) TLDs() []string {
	return c.tlds
}
