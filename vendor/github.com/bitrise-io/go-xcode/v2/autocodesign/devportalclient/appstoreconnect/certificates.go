package appstoreconnect

import (
	"fmt"
	"net/http"
)

// CertificatesEndpoint ...
const CertificatesEndpoint = "certificates"

// ListCertificatesSortOption ...
type ListCertificatesSortOption string

// ListCertificatesSortOptions ...
const (
	ListCertificatesSortOptionDisplayName         ListCertificatesSortOption = "displayName"
	ListCertificatesSortOptionDisplayNameDesc     ListCertificatesSortOption = "-displayName"
	ListCertificatesSortOptionCertificateType     ListCertificatesSortOption = "certificateType"
	ListCertificatesSortOptionCertificateTypeDesc ListCertificatesSortOption = "-certificateType"
	ListCertificatesSortOptionSerialNumber        ListCertificatesSortOption = "serialNumber"
	ListCertificatesSortOptionSerialNumberDesc    ListCertificatesSortOption = "-serialNumber"
	ListCertificatesSortOptionID                  ListCertificatesSortOption = "id"
	ListCertificatesSortOptionIDDesc              ListCertificatesSortOption = "-id"
)

// ListCertificatesOptions ...
type ListCertificatesOptions struct {
	PagingOptions
	FilterSerialNumber    string                     `url:"filter[serialNumber],omitempty"`
	FilterCertificateType CertificateType            `url:"filter[certificateType],omitempty"`
	Sort                  ListCertificatesSortOption `url:"sort,omitempty"`
}

// CertificateType ...
type CertificateType string

// CertificateTypes ...
const (
	Development              CertificateType = "DEVELOPMENT"
	Distribution             CertificateType = "DISTRIBUTION"
	IOSDevelopment           CertificateType = "IOS_DEVELOPMENT"
	IOSDistribution          CertificateType = "IOS_DISTRIBUTION"
	MacDistribution          CertificateType = "MAC_APP_DISTRIBUTION"
	MacInstallerDistribution CertificateType = "MAC_INSTALLER_DISTRIBUTION"
	MacDevelopment           CertificateType = "MAC_APP_DEVELOPMENT"
	DeveloperIDKext          CertificateType = "DEVELOPER_ID_KEXT"
	DeveloperIDApplication   CertificateType = "DEVELOPER_ID_APPLICATION"
)

// CertificateAttributes ...
type CertificateAttributes struct {
	CertificateContent []byte           `json:"certificateContent"`
	DisplayName        string           `json:"displayName"`
	ExpirationDate     string           `json:"expirationDate"`
	Name               string           `json:"name"`
	Platform           BundleIDPlatform `json:"platform"`
	SerialNumber       string           `json:"serialNumber"`
	CertificateType    CertificateType  `json:"certificateType"`
}

// Certificate ...
type Certificate struct {
	Attributes CertificateAttributes `json:"attributes"`
	ID         string                `json:"id"`
	Type       string                `json:"type"`
}

// CertificatesResponse ...
type CertificatesResponse struct {
	Data  []Certificate      `json:"data"`
	Links PagedDocumentLinks `json:"links,omitempty"`
	Meta  PagingInformation  `json:"meta,omitempty"`
}

// ListCertificates ...
func (s ProvisioningService) ListCertificates(opt *ListCertificatesOptions) (*CertificatesResponse, error) {
	if err := opt.UpdateCursor(); err != nil {
		return nil, err
	}

	u, err := addOptions(CertificatesEndpoint, opt)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	r := &CertificatesResponse{}
	if _, err := s.client.Do(req, r); err != nil {
		return nil, err
	}

	return r, nil
}

// FetchCertificate fetch the certificate entity from the
func (s ProvisioningService) FetchCertificate(serialNumber string) (Certificate, error) {
	r, err := s.ListCertificates(&ListCertificatesOptions{
		FilterSerialNumber: serialNumber,
	})
	if err != nil {
		return Certificate{}, fmt.Errorf("failed to fetch certificate (%s): %s", serialNumber, err)
	}

	if len(r.Data) == 0 {
		return Certificate{}, fmt.Errorf("no certificate found with serial %s", serialNumber)
	} else if len(r.Data) > 1 {
		return Certificate{}, fmt.Errorf("multiple certificates found with serial %s: %s", serialNumber, r.Data)
	}
	return r.Data[0], nil
}

// Certificates ...
func (s ProvisioningService) Certificates(relationshipLink string, opt *PagingOptions) (*CertificatesResponse, error) {
	if err := opt.UpdateCursor(); err != nil {
		return nil, err
	}

	u, err := addOptions(relationshipLink, opt)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequestWithRelationshipURL(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	r := &CertificatesResponse{}
	if _, err := s.client.Do(req, r); err != nil {
		return nil, err
	}

	return r, nil
}
