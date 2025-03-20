package appstoreconnect

import (
	"net/http"
)

// BundleIDsEndpoint ...
const BundleIDsEndpoint = "bundleIds"

// ListBundleIDsSortOption ...
type ListBundleIDsSortOption string

// ListBundleIDsSortOptions ...
const (
	ListBundleIDsSortOptionName           ListBundleIDsSortOption = "name"
	ListBundleIDsSortOptionNameDesc       ListBundleIDsSortOption = "-name"
	ListBundleIDsSortOptionPlatform       ListBundleIDsSortOption = "platform"
	ListBundleIDsSortOptionPlatformDesc   ListBundleIDsSortOption = "-platform"
	ListBundleIDsSortOptionIdentifier     ListBundleIDsSortOption = "identifier"
	ListBundleIDsSortOptionIdentifierDesc ListBundleIDsSortOption = "-identifier"
	ListBundleIDsSortOptionSeedId         ListBundleIDsSortOption = "seedId"
	ListBundleIDsSortOptionSeedIdDesc     ListBundleIDsSortOption = "-seedId"
	ListBundleIDsSortOptionID             ListBundleIDsSortOption = "id"
	ListBundleIDsSortOptionIDDesc         ListBundleIDsSortOption = "-id"
)

// ListBundleIDsOptions ...
type ListBundleIDsOptions struct {
	PagingOptions
	FilterIdentifier string                  `url:"filter[identifier],omitempty"`
	FilterName       string                  `url:"filter[name],omitempty"`
	FilterPlatform   BundleIDPlatform        `url:"filter[platform],omitempty"`
	Include          string                  `url:"include,omitempty"`
	Sort             ListBundleIDsSortOption `url:"sort,omitempty"`
}

// PagingInformationPaging ...
type PagingInformationPaging struct {
	Total int `json:"total,omitempty"`
	Limit int `json:"limit,omitempty"`
}

// PagingInformation ...
type PagingInformation struct {
	Paging PagingInformationPaging `json:"paging,omitempty"`
}

// PagedDocumentLinks ...
type PagedDocumentLinks struct {
	First string `json:"first,omitempty"`
	Next  string `json:"next,omitempty"`
	Self  string `json:"self,omitempty"`
}

// BundleIDAttributes ...
type BundleIDAttributes struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
}

// Links ...
type Links struct {
	Related string `json:"related"`
	Self    string `json:"next"`
}

// RelationshipsLinks ...
type RelationshipsLinks struct {
	Links Links `json:"links"`
}

// BundleIDRelationships ...
type BundleIDRelationships struct {
	Profiles     RelationshipsLinks `json:"profiles"`
	Capabilities RelationshipsLinks `json:"bundleIdCapabilities"`
}

// BundleID ...
type BundleID struct {
	Attributes    BundleIDAttributes    `json:"attributes"`
	Relationships BundleIDRelationships `json:"relationships"`

	ID   string `json:"id"`
	Type string `json:"type"`
}

// BundleIdsResponse ...
type BundleIdsResponse struct {
	Data  []BundleID         `json:"data,omitempty"`
	Links PagedDocumentLinks `json:"links,omitempty"`
	Meta  PagingInformation  `json:"meta,omitempty"`
}

// ListBundleIDs ...
func (s ProvisioningService) ListBundleIDs(opt *ListBundleIDsOptions) (*BundleIdsResponse, error) {
	if err := opt.UpdateCursor(); err != nil {
		return nil, err
	}

	u, err := addOptions(BundleIDsEndpoint, opt)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	r := &BundleIdsResponse{}
	if _, err := s.client.Do(req, r); err != nil {
		return nil, err
	}

	return r, err
}

// BundleIDResponse ...
type BundleIDResponse struct {
	Data BundleID `json:"data,omitempty"`
}

// BundleIDCreateRequestDataAttributes ...
type BundleIDCreateRequestDataAttributes struct {
	Identifier string           `json:"identifier"`
	Name       string           `json:"name"`
	Platform   BundleIDPlatform `json:"platform"`
}

// BundleIDCreateRequestData ...
type BundleIDCreateRequestData struct {
	Attributes BundleIDCreateRequestDataAttributes `json:"attributes"`
	Type       string                              `json:"type"`
}

// BundleIDCreateRequest ...
type BundleIDCreateRequest struct {
	Data BundleIDCreateRequestData `json:"data"`
}

// CreateBundleID ...
func (s ProvisioningService) CreateBundleID(body BundleIDCreateRequest) (*BundleIDResponse, error) {
	req, err := s.client.NewRequest(http.MethodPost, BundleIDsEndpoint, body)
	if err != nil {
		return nil, err
	}

	r := &BundleIDResponse{}
	if _, err := s.client.Do(req, r); err != nil {
		return nil, err
	}

	return r, nil
}

// BundleID ...
func (s ProvisioningService) BundleID(relationshipLink string) (*BundleIDResponse, error) {
	req, err := s.client.NewRequestWithRelationshipURL(http.MethodGet, relationshipLink, nil)
	if err != nil {
		return nil, err
	}

	r := &BundleIDResponse{}
	if _, err := s.client.Do(req, r); err != nil {
		return nil, err
	}

	return r, nil
}
