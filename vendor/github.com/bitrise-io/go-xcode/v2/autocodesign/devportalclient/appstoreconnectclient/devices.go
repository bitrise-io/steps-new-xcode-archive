package appstoreconnectclient

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bitrise-io/go-utils/log"

	"github.com/bitrise-io/go-xcode/v2/autocodesign/devportalclient/appstoreconnect"
	"github.com/bitrise-io/go-xcode/v2/devportalservice"
)

// DeviceClient ...
type DeviceClient struct {
	client *appstoreconnect.Client
}

// NewDeviceClient ...
func NewDeviceClient(client *appstoreconnect.Client) *DeviceClient {
	return &DeviceClient{client: client}
}

// ListDevices returns the registered devices on the Apple Developer portal
func (d *DeviceClient) ListDevices(udid string, platform appstoreconnect.DevicePlatform) ([]appstoreconnect.Device, error) {
	var nextPageURL string
	var devices []appstoreconnect.Device
	for {
		response, err := d.client.Provisioning.ListDevices(&appstoreconnect.ListDevicesOptions{
			PagingOptions: appstoreconnect.PagingOptions{
				Limit: 20,
				Next:  nextPageURL,
			},
			FilterUDID:     udid,
			FilterPlatform: platform,
			FilterStatus:   appstoreconnect.Enabled,
		})
		if err != nil {
			var apiError *appstoreconnect.ErrorResponse
			if ok := errors.As(err, &apiError); ok {
				if apiError.IsCursorInvalid() {
					log.Warnf("Cursor is invalid, falling back to listing devices with 400 limit")
					return d.list400Devices(udid, platform)
				}
			}
			return nil, err
		}

		devices = append(devices, response.Data...)

		nextPageURL = response.Links.Next
		if nextPageURL == "" {
			return devices, nil
		}
		if len(devices) >= response.Meta.Paging.Total {
			log.Warnf("All devices fetched, but next page URL is not empty")
			return devices, nil
		}
	}
}

func (d *DeviceClient) list400Devices(udid string, platform appstoreconnect.DevicePlatform) ([]appstoreconnect.Device, error) {
	devicesByID := map[string]appstoreconnect.Device{}
	var totalCount int
	for _, sort := range []appstoreconnect.ListDevicesSortOption{appstoreconnect.ListDevicesSortOptionID, appstoreconnect.ListDevicesSortOptionIDDesc} {
		response, err := d.client.Provisioning.ListDevices(&appstoreconnect.ListDevicesOptions{
			PagingOptions: appstoreconnect.PagingOptions{
				Limit: 200,
			},
			FilterUDID:     udid,
			FilterPlatform: platform,
			FilterStatus:   appstoreconnect.Enabled,
			Sort:           sort,
		})
		if err != nil {
			return nil, err
		}

		for _, responseDevice := range response.Data {
			devicesByID[responseDevice.ID] = responseDevice
		}

		if totalCount == 0 {
			totalCount = response.Meta.Paging.Total
		}
	}

	if totalCount > 0 && totalCount > 400 {
		log.Warnf("More than 400 devices (%d) found", totalCount)
	}

	var devices []appstoreconnect.Device
	for _, device := range devicesByID {
		devices = append(devices, device)
	}

	return devices, nil
}

// RegisterDevice ...
func (d *DeviceClient) RegisterDevice(testDevice devportalservice.TestDevice) (*appstoreconnect.Device, error) {
	// The API seems to recognize existing devices even with different casing and '-' separator removed.
	// The Developer Portal UI does not let adding devices with unexpected casing or separators removed.
	// Did not fully validate the ability to add devices with changed casing (or '-' removed) via the API, so passing the UDID through unchanged.
	req := appstoreconnect.DeviceCreateRequest{
		Data: appstoreconnect.DeviceCreateRequestData{
			Attributes: appstoreconnect.DeviceCreateRequestDataAttributes{
				Name:     "Bitrise test device",
				Platform: appstoreconnect.IOS,
				UDID:     testDevice.DeviceID,
			},
			Type: "devices",
		},
	}

	registeredDevice, err := d.client.Provisioning.RegisterNewDevice(req)
	if err != nil {
		var respErr *appstoreconnect.ErrorResponse
		if ok := errors.As(err, &respErr); ok {
			if respErr.Response != nil && respErr.Response.StatusCode == http.StatusConflict {
				return nil, appstoreconnect.DeviceRegistrationError{
					Reason: fmt.Sprintf("%v", err),
				}
			}
		}

		return nil, err
	}

	return &registeredDevice.Data, nil
}
