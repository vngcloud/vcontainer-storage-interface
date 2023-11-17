package vcontainer

import (
	snapV1Obj "github.com/vngcloud/vcontainer-sdk/vcontainer/services/blockstorage/v1/snapshot/obj"
	snapV2Obj "github.com/vngcloud/vcontainer-sdk/vcontainer/services/blockstorage/v2/snapshot/obj"
	"github.com/vngcloud/vcontainer-storage-interface/csi/utils/metadata"

	bstgObj "github.com/vngcloud/vcontainer-sdk/vcontainer/services/blockstorage/v2/volume/obj"
	comObj "github.com/vngcloud/vcontainer-sdk/vcontainer/services/compute/v2/server/obj"
)

type IVContainer interface {
	GetMetadataOpts() metadata.Opts
	SetupPortalInfo(metadata metadata.IMetadata) error
	ListVolumes(limit int, startingToken string) ([]*bstgObj.Volume, string, error)
	GetVolumesByName(n string) ([]*bstgObj.Volume, error)
	GetVolume(volumeID string) (*bstgObj.Volume, error)
	CreateVolume(name string, size uint64, vtype, availability string, snapshotID string, sourcevolID string, tags *map[string]string) (*bstgObj.Volume, error)
	DeleteVolume(volID string) error
	GetInstanceByID(instanceID string) (*comObj.Server, error)
	AttachVolume(instanceID, volumeID string) (string, error)
	GetAttachmentDiskPath(instanceID, volumeID string) (string, error)
	WaitDiskAttached(instanceID string, volumeID string) error
	DetachVolume(instanceID, volumeID string) error
	WaitDiskDetached(instanceID string, volumeID string) error
	ExpandVolume(volumeTypeID, volumeID string, newSize uint64) error
	WaitVolumeTargetStatus(volumeID string, tStatus []string) error
	GetMaxVolLimit() int64
	GetBlockStorageOpts() BlockStorageOpts
	ListSnapshots(page string, size int, volumeID, status, name string) ([]*snapV1Obj.Snapshot, string, error)
	GetSnapshotByID(snapshotID string) (*snapV1Obj.Snapshot, error)
	CreateSnapshot(name, volID string) (*snapV2Obj.Snapshot, error)
	WaitSnapshotReady(snapshotID string) error
	DeleteSnapshot(volumeID, snapshotID string) error
	GetMappingVolume(volumeID string) (string, error)
}
