package appstoreconnectclient

import (
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-xcode/certificateutil"
	"github.com/bitrise-io/go-xcode/v2/autocodesign"
	"github.com/bitrise-io/go-xcode/v2/autocodesign/devportalclient/appstoreconnect"
)

// CertificateSource ...
type CertificateSource struct {
	client *appstoreconnect.Client
}

// NewCertificateSource ...
func NewCertificateSource(client *appstoreconnect.Client) *CertificateSource {
	return &CertificateSource{
		client: client,
	}
}

// QueryCertificateBySerial ...
func (s *CertificateSource) QueryCertificateBySerial(serial big.Int) (autocodesign.Certificate, error) {
	response, err := s.client.Provisioning.FetchCertificate(serial.Text(16))
	if err != nil {
		return autocodesign.Certificate{}, err
	}

	certs, err := parseCertificatesResponse([]appstoreconnect.Certificate{response})
	if err != nil {
		return autocodesign.Certificate{}, err
	}

	return certs[0], nil
}

// QueryAllIOSCertificates returns all iOS certificates from App Store Connect API
func (s *CertificateSource) QueryAllIOSCertificates() (map[appstoreconnect.CertificateType][]autocodesign.Certificate, error) {
	typeToCertificates := map[appstoreconnect.CertificateType][]autocodesign.Certificate{}

	for _, certType := range []appstoreconnect.CertificateType{appstoreconnect.Development, appstoreconnect.IOSDevelopment, appstoreconnect.Distribution, appstoreconnect.IOSDistribution} {
		certs, err := queryCertificatesByType(s.client, certType)
		if err != nil {
			return map[appstoreconnect.CertificateType][]autocodesign.Certificate{}, err
		}
		typeToCertificates[certType] = certs
	}

	return typeToCertificates, nil
}

func parseCertificatesResponse(response []appstoreconnect.Certificate) ([]autocodesign.Certificate, error) {
	var certifacteInfos []autocodesign.Certificate
	for _, resp := range response {
		if resp.Type == "certificates" {
			cert, err := x509.ParseCertificate(resp.Attributes.CertificateContent)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %s", err)
			}

			certInfo := certificateutil.NewCertificateInfo(*cert, nil)

			certifacteInfos = append(certifacteInfos, autocodesign.Certificate{
				CertificateInfo: certInfo,
				ID:              resp.ID,
			})
		}
	}

	return certifacteInfos, nil
}

func queryCertificatesByType(client *appstoreconnect.Client, certificateType appstoreconnect.CertificateType) ([]autocodesign.Certificate, error) {
	nextPageURL := ""
	var certificates []appstoreconnect.Certificate
	for {
		response, err := client.Provisioning.ListCertificates(&appstoreconnect.ListCertificatesOptions{
			PagingOptions: appstoreconnect.PagingOptions{
				Limit: 20,
				Next:  nextPageURL,
			},
			FilterCertificateType: certificateType,
		})
		if err != nil {
			var apiError *appstoreconnect.ErrorResponse
			if ok := errors.As(err, &apiError); ok {
				if apiError.IsCursorInvalid() {
					log.Warnf("Cursor is invalid, falling back to listing certificates with 400 limit")
					return list400Certificates(client, certificateType)
				}
			}
			return nil, err
		}

		certificates = append(certificates, response.Data...)

		nextPageURL = response.Links.Next
		if nextPageURL == "" {
			return parseCertificatesResponse(certificates)
		}
		if len(certificates) >= response.Meta.Paging.Total {
			log.Warnf("All certificates fetched, but next page URL is not empty")
			return parseCertificatesResponse(certificates)
		}
	}
}

func list400Certificates(client *appstoreconnect.Client, certificateType appstoreconnect.CertificateType) ([]autocodesign.Certificate, error) {
	certificatesByID := map[string]appstoreconnect.Certificate{}
	var totalCount int
	for _, sort := range []appstoreconnect.ListCertificatesSortOption{appstoreconnect.ListCertificatesSortOptionID, appstoreconnect.ListCertificatesSortOptionIDDesc} {
		response, err := client.Provisioning.ListCertificates(&appstoreconnect.ListCertificatesOptions{
			PagingOptions: appstoreconnect.PagingOptions{
				Limit: 200,
			},
			FilterCertificateType: certificateType,
			Sort:                  sort,
		})
		if err != nil {
			return nil, err
		}

		for _, responseCertificate := range response.Data {
			certificatesByID[responseCertificate.ID] = responseCertificate
		}

		if totalCount == 0 {
			totalCount = response.Meta.Paging.Total
		}
	}

	if totalCount > 0 && totalCount > 400 {
		log.Warnf("More than 400 certificates (%d) found", totalCount)
	}

	var certificates []appstoreconnect.Certificate
	for _, certificate := range certificatesByID {
		certificates = append(certificates, certificate)
	}

	return parseCertificatesResponse(certificates)
}
