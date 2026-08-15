package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	defaultTTL = 1800
	minTTL     = 60
	maxTTL     = 60000
)

type Server struct {
	client    *NamecheapClient
	cfg       *Config
	startedAt time.Time
}

func NewServer(cfg *Config) (*Server, error) {
	client := NewNamecheapClient(cfg)
	if err := client.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize Namecheap client: %w", err)
	}
	return &Server{
		client:    client,
		cfg:       cfg,
		startedAt: time.Now(),
	}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.NegotiateHandler)
	mux.HandleFunc(UrlRecords, s.RecordsHandler)
	mux.HandleFunc(UrlAdjustEndpoints, s.AdjustEndpointsHandler)
	mux.HandleFunc("/healthz", s.HealthzHandler)
	return mux
}

func (s *Server) NegotiateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusBadRequest)
		return
	}

	df := s.cfg.GetDomainFilter()
	w.Header().Set(ContentTypeHeader, MediaTypeFormatAndVersion)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(df); err != nil {
		log.Printf("failed to encode domain filter: %v", err)
	}
}

func (s *Server) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetRecords(w, r)
	case http.MethodPost:
		s.handleApplyChanges(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusBadRequest)
	}
}

func (s *Server) handleGetRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.getRecords(r.Context())
	if err != nil {
		log.Printf("failed to get records: %v", err)
		http.Error(w, "failed to get records", http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentTypeHeader, MediaTypeFormatAndVersion)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(records); err != nil {
		log.Printf("failed to encode records: %v", err)
	}
}

func (s *Server) handleApplyChanges(w http.ResponseWriter, r *http.Request) {
	var changes Changes
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		log.Printf("failed to decode changes: %v", err)
		http.Error(w, "failed to decode changes", http.StatusBadRequest)
		return
	}

	if err := s.applyChanges(r.Context(), &changes); err != nil {
		log.Printf("failed to apply changes: %v", err)
		http.Error(w, "failed to apply changes", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) AdjustEndpointsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusBadRequest)
		return
	}

	var endpoints []*Endpoint
	if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {
		log.Printf("failed to decode endpoints: %v", err)
		http.Error(w, "failed to decode endpoints", http.StatusBadRequest)
		return
	}

	w.Header().Set(ContentTypeHeader, MediaTypeFormatAndVersion)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(endpoints); err != nil {
		log.Printf("failed to encode endpoints: %v", err)
	}
}

func (s *Server) HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ok",
		"started_at": s.startedAt.Format(time.RFC3339),
		"tld_count":  s.client.TLDCount(),
	})
}

func (s *Server) getRecords(ctx context.Context) ([]*Endpoint, error) {
	domains, err := s.client.GetDomains()
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	allEndpoints := []*Endpoint{}

	for _, domain := range domains {
		if !s.cfg.DomainFilterMatch(domain.Name) {
			continue
		}

		sld, tld, _ := s.client.SplitDomain(domain.Name)
		if sld == "" || tld == "" {
			continue
		}

		hosts, err := s.client.GetHosts(sld, tld)
		if err != nil {
			log.Printf("failed to get hosts for %s: %v", domain.Name, err)
			continue
		}

		for _, host := range hosts {
			ep := s.hostToEndpoint(host, domain.Name)
			if ep != nil {
				allEndpoints = append(allEndpoints, ep)
			}
		}
	}

	return allEndpoints, nil
}

func (s *Server) hostToEndpoint(host Host, domainName string) *Endpoint {
	recordType := strings.ToUpper(strings.TrimSpace(host.Type))

	if recordType == "URL" || recordType == "URL301" || recordType == "FRAME" || recordType == "MXE" {
		return nil
	}

	name := strings.TrimSpace(host.Name)
	if name == "@" || name == "" {
		dnsName := domainName
		return s.buildEndpoint(dnsName, recordType, host)
	}

	dnsName := name + "." + domainName
	return s.buildEndpoint(dnsName, recordType, host)
}

func (s *Server) buildEndpoint(dnsName, recordType string, host Host) *Endpoint {
	ttl := ParseTTL(host.TTL)
	if ttl == 0 {
		ttl = defaultTTL
	}

	ep := &Endpoint{
		DNSName:    dnsName,
		RecordType: recordType,
		RecordTTL:  int64(ttl),
	}

	switch recordType {
	case "MX":
		if host.MXPref != "" {
			ep.Targets = []string{host.MXPref + " " + host.Address}
		} else {
			ep.Targets = []string{host.Address}
		}
	default:
		ep.Targets = []string{host.Address}
	}

	return ep
}

func (s *Server) applyChanges(ctx context.Context, changes *Changes) error {
	changesByDomain := s.groupChangesByDomain(changes)

	for domainName, domainChanges := range changesByDomain {
		if err := s.applyChangesForDomain(ctx, domainName, domainChanges); err != nil {
			return fmt.Errorf("failed to apply changes for domain %s: %w", domainName, err)
		}
	}

	return nil
}

type domainChanges struct {
	create    []*Endpoint
	updateNew []*Endpoint
	updateOld []*Endpoint
	delete    []*Endpoint
}

func (s *Server) groupChangesByDomain(changes *Changes) map[string]*domainChanges {
	result := make(map[string]*domainChanges)

	for _, ep := range changes.Create {
		domain := s.extractDomain(ep.DNSName)
		if domain == "" {
			continue
		}
		dc := s.getOrCreateDomainChanges(result, domain)
		dc.create = append(dc.create, ep)
	}
	for _, ep := range changes.UpdateNew {
		domain := s.extractDomain(ep.DNSName)
		if domain == "" {
			continue
		}
		dc := s.getOrCreateDomainChanges(result, domain)
		dc.updateNew = append(dc.updateNew, ep)
	}
	for _, ep := range changes.UpdateOld {
		domain := s.extractDomain(ep.DNSName)
		if domain == "" {
			continue
		}
		dc := s.getOrCreateDomainChanges(result, domain)
		dc.updateOld = append(dc.updateOld, ep)
	}
	for _, ep := range changes.Delete {
		domain := s.extractDomain(ep.DNSName)
		if domain == "" {
			continue
		}
		dc := s.getOrCreateDomainChanges(result, domain)
		dc.delete = append(dc.delete, ep)
	}

	return result
}

func (s *Server) getOrCreateDomainChanges(m map[string]*domainChanges, domain string) *domainChanges {
	if dc, ok := m[domain]; ok {
		return dc
	}
	dc := &domainChanges{}
	m[domain] = dc
	return dc
}

func (s *Server) extractDomain(dnsName string) string {
	dnsName = strings.TrimSuffix(dnsName, ".")
	parts := strings.Split(dnsName, ".")
	if len(parts) < 2 {
		return ""
	}

	for _, knownTLD := range s.client.TLDs() {
		tldLabels := strings.Split(knownTLD, ".")
		if len(parts) < len(tldLabels) {
			continue
		}
		suffix := strings.Join(parts[len(parts)-len(tldLabels):], ".")
		if suffix == knownTLD {
			return strings.Join(parts[len(parts)-len(tldLabels)-1:], ".")
		}
	}

	return strings.Join(parts[len(parts)-2:], ".")
}

func (s *Server) applyChangesForDomain(ctx context.Context, domainName string, dc *domainChanges) error {
	sld, tld, _ := s.client.SplitDomain(domainName)
	if sld == "" || tld == "" {
		return fmt.Errorf("invalid domain name: %s", domainName)
	}

	currentHosts, err := s.client.GetHosts(sld, tld)
	if err != nil {
		return fmt.Errorf("failed to get current hosts: %w", err)
	}

	hostMap := make(map[string]map[string]*Host)
	for i := range currentHosts {
		h := &currentHosts[i]
		if hostMap[h.Name] == nil {
			hostMap[h.Name] = make(map[string]*Host)
		}
		hostMap[h.Name][h.Type] = h
	}

	for _, ep := range dc.updateOld {
		s.removeEndpointFromHosts(hostMap, ep, domainName)
	}
	for _, ep := range dc.delete {
		s.removeEndpointFromHosts(hostMap, ep, domainName)
	}
	for _, ep := range dc.create {
		s.applyEndpointToHosts(hostMap, ep, domainName)
	}
	for _, ep := range dc.updateNew {
		s.applyEndpointToHosts(hostMap, ep, domainName)
	}

	var finalHosts []Host
	for name, byType := range hostMap {
		for _, h := range byType {
			h.Name = name
			finalHosts = append(finalHosts, *h)
		}
	}

	sort.Slice(finalHosts, func(i, j int) bool {
		if finalHosts[i].Name != finalHosts[j].Name {
			return finalHosts[i].Name < finalHosts[j].Name
		}
		return finalHosts[i].Type < finalHosts[j].Type
	})

	if err := s.client.SetHosts(sld, tld, finalHosts); err != nil {
		return fmt.Errorf("failed to set hosts: %w", err)
	}

	return nil
}

func (s *Server) applyEndpointToHosts(hostMap map[string]map[string]*Host, ep *Endpoint, domainName string) {
	hostname := s.endpointHostname(ep.DNSName, domainName)
	recordType := strings.ToUpper(ep.RecordType)

	ttl := int(ep.RecordTTL)
	if ttl == 0 {
		ttl = defaultTTL
	}
	if ttl < minTTL {
		ttl = minTTL
	}
	if ttl > maxTTL {
		ttl = maxTTL
	}

	host := &Host{
		Name:    hostname,
		Type:    recordType,
		TTL:     fmt.Sprintf("%d", ttl),
	}

	switch recordType {
	case "MX":
		targets := ep.Targets
		if len(targets) > 0 {
			parts := strings.SplitN(targets[0], " ", 2)
			if len(parts) == 2 {
				host.MXPref = parts[0]
				host.Address = parts[1]
			} else {
				host.Address = targets[0]
			}
		}
	case "A", "AAAA", "CNAME", "TXT", "NS":
		if len(ep.Targets) > 0 {
			host.Address = ep.Targets[0]
		}
	default:
		return
	}

	if hostMap[hostname] == nil {
		hostMap[hostname] = make(map[string]*Host)
	}
	hostMap[hostname][recordType] = host
}

func (s *Server) removeEndpointFromHosts(hostMap map[string]map[string]*Host, ep *Endpoint, domainName string) {
	hostname := s.endpointHostname(ep.DNSName, domainName)
	recordType := strings.ToUpper(ep.RecordType)
	if byType, ok := hostMap[hostname]; ok {
		delete(byType, recordType)
	}
}

func (s *Server) endpointHostname(dnsName, domainName string) string {
	dnsName = strings.TrimSuffix(dnsName, ".")
	if dnsName == domainName {
		return "@"
	}
	if strings.HasSuffix(dnsName, "."+domainName) {
		return strings.TrimSuffix(dnsName, "."+domainName)
	}
	return dnsName
}
