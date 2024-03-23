package metadata

import "github.com/vngcloud/vcontainer-storage-interface/csi/utils"

type IMetadata interface {
	GetInstanceID() (string, error)
	GetAvailabilityZone() (string, error)
	GetProjectID() (string, error)
}

type (
	Opts struct {
		SearchOrder    string           `gcfg:"search-order"`    // will be configDriver
		RequestTimeout utils.MyDuration `gcfg:"request-timeout"` // will be 0
	}

	Metadata struct {
		UUID             string           `json:"uuid"`
		Name             string           `json:"name"`
		AvailabilityZone string           `json:"availability_zone"`
		ProjectID        string           `json:"project_id"`
		Devices          []DeviceMetadata `json:"devices,omitempty"`
		Meta             Meta             `json:"meta,omitempty"`
		// ... and other fields we don't care about.  Expand as necessary.
	}

	Meta struct {
		Product    string `json:"product,omitempty"`
		PortalUUID string `json:"portal_uuid,omitempty"`
		ProjectID  string `json:"project_id,omitempty"`
	}

	DeviceMetadata struct {
		Type    string `json:"type"`
		Bus     string `json:"bus,omitempty"`
		Serial  string `json:"serial,omitempty"`
		Address string `json:"address,omitempty"`
		// ... and other fields.
	}
)
