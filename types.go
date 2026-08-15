package webhook

import "encoding/json"

const (
	MediaTypeFormatAndVersion = "application/external.dns.webhook+json;version=1"
	ContentTypeHeader         = "Content-Type"
	AcceptHeader              = "Accept"
	UrlRecords                = "/records"
	UrlAdjustEndpoints        = "/adjustendpoints"
)

type DomainFilter struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type ProviderSpecificProperty struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type Endpoint struct {
	DNSName          string                     `json:"dnsName,omitempty"`
	Targets          []string                   `json:"targets,omitempty"`
	RecordType       string                     `json:"recordType,omitempty"`
	SetIdentifier    string                     `json:"setIdentifier,omitempty"`
	RecordTTL        int64                      `json:"recordTTL,omitempty"`
	Labels           map[string]string          `json:"labels,omitempty"`
	ProviderSpecific []ProviderSpecificProperty `json:"providerSpecific,omitempty"`
}

type Changes struct {
	Create    []*Endpoint `json:"create,omitempty"`
	UpdateOld []*Endpoint `json:"updateOld,omitempty"`
	UpdateNew []*Endpoint `json:"updateNew,omitempty"`
	Delete    []*Endpoint `json:"delete,omitempty"`
}

func MarshalDomainFilter(df DomainFilter) ([]byte, error) {
	return json.Marshal(df)
}

func UnmarshalChanges(data []byte) (*Changes, error) {
	var changes Changes
	if err := json.Unmarshal(data, &changes); err != nil {
		return nil, err
	}
	return &changes, nil
}

func MarshalEndpoints(endpoints []*Endpoint) ([]byte, error) {
	return json.Marshal(endpoints)
}

func UnmarshalEndpoints(data []byte) ([]*Endpoint, error) {
	var endpoints []*Endpoint
	if err := json.Unmarshal(data, &endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}
