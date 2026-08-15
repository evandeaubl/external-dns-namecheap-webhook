package webhook

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	DefaultWebhookPort   = ":8888"
	DefaultHealthzPort   = ":8080"
	DefaultSandboxURL    = "https://api.sandbox.namecheap.com/xml.response"
	DefaultProductionURL = "https://api.namecheap.com/xml.response"
	DefaultRequestTTL    = 60 * time.Second
)

type Config struct {
	APIUser      string
	APIKey       string
	Username     string
	ClientIP     string
	Production   bool
	DomainFilters []string
	ListenAddr   string
	HealthzAddr  string
	RequestTTL   time.Duration
}

func ParseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.APIUser, "api-user", "", "Namecheap API user (env: NAMECHEAP_API_USER)")
	flag.StringVar(&cfg.APIKey, "api-key", "", "Namecheap API key (env: NAMECHEAP_API_KEY)")
	flag.StringVar(&cfg.Username, "username", "", "Namecheap username (env: NAMECHEAP_USERNAME)")
	flag.StringVar(&cfg.ClientIP, "client-ip", "", "Client IP for Namecheap API (env: NAMECHEAP_CLIENT_IP)")
	flag.BoolVar(&cfg.Production, "production", false, "Use Namecheap production environment (default: sandbox) (env: NAMECHEAP_PRODUCTION)")
	flag.Var((*stringSliceFlag)(&cfg.DomainFilters), "domain-filter", "Comma-separated list of domain filters (env: NAMECHEAP_DOMAIN_FILTER)")
	flag.StringVar(&cfg.ListenAddr, "listen-address", DefaultWebhookPort, "Address to listen on for webhook server (env: LISTEN_ADDRESS)")
	flag.StringVar(&cfg.HealthzAddr, "healthz-address", DefaultHealthzPort, "Address to listen on for health/metrics server (env: HEALTHZ_ADDRESS)")
	flag.DurationVar(&cfg.RequestTTL, "request-ttl", DefaultRequestTTL, "Timeout for Namecheap API requests (env: REQUEST_TTL)")

	flag.Parse()

	envOrFlag("NAMECHEAP_API_USER", &cfg.APIUser)
	envOrFlag("NAMECHEAP_API_KEY", &cfg.APIKey)
	envOrFlag("NAMECHEAP_USERNAME", &cfg.Username)
	envOrFlag("NAMECHEAP_CLIENT_IP", &cfg.ClientIP)
	envOrFlag("NAMECHEAP_PRODUCTION", &cfg.Production)
	envOrFlag("NAMECHEAP_DOMAIN_FILTER", (*stringSliceFlag)(&cfg.DomainFilters))
	envOrFlag("LISTEN_ADDRESS", &cfg.ListenAddr)
	envOrFlag("HEALTHZ_ADDRESS", &cfg.HealthzAddr)
	envOrFlag("REQUEST_TTL", &cfg.RequestTTL)

	return cfg
}

func envOrFlag(envKey string, target interface{}) {
	val := os.Getenv(envKey)
	if val == "" {
		return
	}
	switch t := target.(type) {
	case *string:
		*t = val
	case *bool:
		*t = strings.EqualFold(val, "true") || val == "1"
	case *time.Duration:
		d, err := time.ParseDuration(val)
		if err == nil {
			*t = d
		}
	case *stringSliceFlag:
		*t = parseStringSlice(val)
	}
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = parseStringSlice(value)
	return nil
}

func parseStringSlice(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (c *Config) APIURL() string {
	if c.Production {
		return DefaultProductionURL
	}
	return DefaultSandboxURL
}

func (c *Config) Validate() error {
	if c.APIUser == "" {
		return fmt.Errorf("api-user is required (set via --api-user or NAMECHEAP_API_USER env var)")
	}
	if c.APIKey == "" {
		return fmt.Errorf("api-key is required (set via --api-key or NAMECHEAP_API_KEY env var)")
	}
	if c.Username == "" {
		return fmt.Errorf("username is required (set via --username or NAMECHEAP_USERNAME env var)")
	}
	if c.ClientIP == "" {
		return fmt.Errorf("client-ip is required (set via --client-ip or NAMECHEAP_CLIENT_IP env var)")
	}
	return nil
}

func (c *Config) GetDomainFilter() DomainFilter {
	if len(c.DomainFilters) == 0 {
		return DomainFilter{}
	}
	return DomainFilter{Include: c.DomainFilters}
}

func (c *Config) DomainFilterMatch(domain string) bool {
	if len(c.DomainFilters) == 0 {
		return true
	}
	for _, f := range c.DomainFilters {
		if matchDomainFilter(f, domain) {
			return true
		}
	}
	return false
}

func matchDomainFilter(filter, domain string) bool {
	if filter == "" {
		return true
	}
	if strings.HasPrefix(filter, ".") {
		return strings.HasSuffix(domain, filter) || domain == strings.TrimPrefix(filter, ".")
	}
	if strings.HasSuffix(domain, "."+filter) {
		return true
	}
	return domain == filter
}

func ParseTTL(val string) int {
	if val == "" {
		return 0
	}
	var ttl int
	if _, err := fmt.Sscanf(val, "%d", &ttl); err != nil {
		return 0
	}
	return ttl
}
