package driver

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/vngcloud/vcontainer-sdk/vcontainer/services/blockstorage/v1/snapshot/obj"
	obj2 "github.com/vngcloud/vcontainer-sdk/vcontainer/services/blockstorage/v2/snapshot/obj"
	"github.com/vngcloud/vcontainer-storage-interface/csi/utils"
	"github.com/vngcloud/vcontainer-storage-interface/csi/utils/metadata"
	"github.com/vngcloud/vcontainer-storage-interface/csi/vcontainer/vcontainer"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"
	"time"
)

type controllerServer struct {
	Driver   *Driver
	Metadata metadata.IMetadata
	Cloud    vcontainer.IVContainer
}

func (s *controllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.V(5).Infof("ListVolumes; called with request %+v", req)

	if req.MaxEntries < 0 {
		return nil, status.Error(
			codes.InvalidArgument,
			fmt.Sprintf("ListVolumes; invalid max entries request %v, must not be negative ", req.MaxEntries))
	}

	maxEntries := int(req.MaxEntries)
	vlist, nextPageToken, err := s.Cloud.ListVolumes(maxEntries, req.StartingToken)
	if err != nil {
		klog.Errorf("ListVolumes; failed to list volumes; ERR: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("ListVolumes failed with error %v", err))
	}

	ventries := make([]*csi.ListVolumesResponse_Entry, len(vlist))
	for i, v := range vlist {
		ventry := &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      v.VolumeId,
				CapacityBytes: int64(v.Size * (1024 ^ 3)),
			},
		}

		csiStatus := new(csi.ListVolumesResponse_VolumeStatus)
		if isAttachment(v.VmId) {
			csiStatus.PublishedNodeIds = []string{*v.VmId}
		}

		ventry.Status = csiStatus
		ventries[i] = ventry
	}

	klog.V(4).Infof("ListVolumes; completed with %d entries and next page is %s.", len(ventries), nextPageToken)
	return &csi.ListVolumesResponse{
		Entries:   ventries,
		NextToken: nextPageToken,
	}, nil
}

func (s *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).Infof("CreateVolume; called with request %+v", protosanitizer.StripSecrets(*req))

	volName := req.GetName()                       // get the volume name
	volCapabilities := req.GetVolumeCapabilities() // get the volume capabilities

	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume name is required")
	}

	if volCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities are required")
	}

	// set the default volume size if 1 GiB
	volSizeBytes := int64(1 * 1024 * 1024 * 1024)

	// get the volume size that user provided
	if req.GetCapacityRange() != nil {
		volSizeBytes = int64(req.GetCapacityRange().GetRequiredBytes())
	}

	// round up the volume size to GiB
	volSizeGB := uint64(utils.RoundUpSize(volSizeBytes, 1024*1024*1024))

	// get the volume type of the StorageClass
	volType := req.GetParameters()["type"]

	// First check if volAvailability is already specified, if not get preferred from Topology
	// Required, incase vol AZ is different from node AZ
	volAvailability := req.GetParameters()["availability"]

	// currently, my cloud does not support :(
	if volAvailability == "" {
		// Check from Topology
		if req.GetAccessibilityRequirements() != nil {
			volAvailability = utils.GetAZFromTopology(topologyKey, req.GetAccessibilityRequirements())
		}
	}

	volumes, err := s.Cloud.GetVolumesByName(volName)
	if err != nil {
		klog.Errorf("CreateVolume; failed to get volumes by name %s; ERR: %v", volName, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get volumes by name; ERR: %v", err))
	}

	if len(volumes) == 1 {
		if volSizeGB != volumes[0].Size {
			klog.Errorf("CreateVolume; volume %s already exists with different size %d", volName, volumes[0].Size)
			return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("volume %s already exists with different size %d", volName, volumes[0].Size))
		}

		klog.V(4).Infof("CreateVolume; volume %s already exists with same size %d", volName, volumes[0].Size)
		return getCreateVolumeResponse(volumes[0]), nil
	} else if len(volumes) > 1 {
		klog.V(3).Infof("CreateVolume; volume %s already exists with different size %d", volName, volumes[0].Size)
		return nil, status.Error(codes.Internal, "Multiple volumes reported with same name")
	}

	// volume creation
	properties := map[string]string{
		vContainerCSIClusterIDKey: "vcontainer-kubernetes-cluster",
	}
	//Tag volume with metadata if present: https://github.com/kubernetes-csi/external-provisioner/pull/399
	for _, mKey := range []string{"csi.storage.k8s.io/pvc/name", "csi.storage.k8s.io/pvc/namespace", "csi.storage.k8s.io/pv/name"} {
		if v, ok := req.Parameters[mKey]; ok {
			properties[mKey] = v
		}
	}

	createdVol, err := s.Cloud.CreateVolume(volName, volSizeGB, volType, volAvailability, "", "", &properties)

	if err != nil {
		klog.Errorf("CreateVolume; failed to create volume %s; ERR: %v", volName, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create volume; ERR: %v", err))
	}

	klog.V(4).Infof("CreateVolume; volume %s (%d GiB) created successfully", volName, volSizeGB)

	return getCreateVolumeResponse(createdVol), nil
}

func (s *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume; called with request %+v", req)

	volID := req.GetVolumeId()
	if volID == "" {
		klog.Errorf("DeleteVolume; Volume ID is required")
		return nil, status.Error(codes.InvalidArgument, "Volume ID is required")
	}

	vol, err := s.Cloud.GetVolume(volID)
	if err != nil {
		klog.Errorf("DeleteVolume; failed to get volume %s; ERR: %v", volID, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get volume; ERR: %v", err))
	}

	if vol.PersistentVolume != true {
		klog.Errorf("DeleteVolume; volume %s is not a persistent volume", volID)
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("volume %s is not a persistent volume", volID))
	}

	err = s.Cloud.DeleteVolume(volID)
	if err != nil {
		klog.Errorf("DeleteVolume; failed to delete volume %s; ERR: %v", volID, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to delete volume; ERR: %v", err))
	}

	klog.V(4).Infof("DeleteVolume; volume %s deleted successfully", volID)

	return &csi.DeleteVolumeResponse{}, nil
}

func (s *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (result *csi.ControllerPublishVolumeResponse, err error) {
	klog.V(4).Infof("ControllerPublishVolume; called with request %+v", req)

	instanceID := req.GetNodeId()
	volumeID := req.GetVolumeId()
	volumeCapability := req.GetVolumeCapability()

	if instanceID == "" {
		klog.Errorf("ControllerPublishVolume; Node ID is required")
		return nil, status.Error(codes.InvalidArgument, "Node ID is required")
	}

	if volumeID == "" {
		klog.Errorf("ControllerPublishVolume; Volume ID is required")
		return nil, status.Error(codes.InvalidArgument, "Volume ID is required")
	}

	if volumeCapability == nil {
		klog.Errorf("ControllerPublishVolume; Volume capability is required")
		return nil, status.Error(codes.InvalidArgument, "Volume capability is required")
	}

	_, err = s.Cloud.GetVolume(volumeID)
	if err != nil {
		klog.Errorf("ControllerPublishVolume; failed to get volume %s; ERR: %v", volumeID, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get volume; ERR: %v", err))
	}

	//_, err = s.Cloud.GetInstanceByID(instanceID)
	//if err != nil {
	//	klog.Errorf("ControllerPublishVolume; failed to get instance %s; ERR: %v", instanceID, err)
	//	return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get instance; ERR: %v", err))
	//}

	_, err = s.Cloud.AttachVolume(instanceID, volumeID)
	if err != nil {
		klog.Errorf("ControllerPublishVolume; failed to attach volume %s to instance %s; ERR: %v", volumeID, instanceID, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to attach volume; ERR: %v", err))
	}

	err = s.Cloud.WaitDiskAttached(instanceID, volumeID)
	if err != nil {
		klog.Errorf("ControllerPublishVolume; failed to wait disk attached to instance %s; ERR: %v", instanceID, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to wait disk attached; ERR: %v", err))
	}

	_, err = s.Cloud.GetAttachmentDiskPath(instanceID, volumeID)
	if err != nil {
		klog.Errorf("ControllerPublishVolume; failed to get device path for volume %s; ERR: %v", volumeID, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get device path; ERR: %v", err))
	}

	klog.V(4).Infof("ControllerPublishVolume; volume %s attached to instance %s successfully", volumeID, instanceID)
	return &csi.ControllerPublishVolumeResponse{
		//PublishContext: map[string]string{
		//	"DevicePath": devicePath,
		//},
	}, nil
}

func (s *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerUnpublishVolume; called with request %+v", req)

	instanceID := req.GetNodeId()
	volumeID := req.GetVolumeId()

	if volumeID == "" {
		klog.Errorf("ControllerUnpublishVolume; Volume ID is required")
		return nil, status.Error(codes.InvalidArgument, "Volume ID is required")
	}

	err := s.Cloud.DetachVolume(instanceID, volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to detach volume; ERR: %v", err))
	}

	err = s.Cloud.WaitDiskDetached(instanceID, volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to wait disk detached; ERR: %v", err))
	}

	klog.V(4).Infof("ControllerUnpublishVolume; volume %s detached from instance %s successfully", volumeID, instanceID)
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (s *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	klog.V(4).Infof("CreateSnapshot: called with args %+v", protosanitizer.StripSecrets(*req))

	name := req.Name
	volumeID := req.GetSourceVolumeId()

	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "Snapshot name must be provided in CreateSnapshot request")
	}

	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumeID must be provided in CreateSnapshot request")
	}

	// Verify a snapshot with the provided name doesn't already exist for this tenant
	snapshots, _, err := s.Cloud.ListSnapshots("1", 10, volumeID, "", "name")
	if err != nil {
		klog.Errorf("Failed to query for existing Snapshot during CreateSnapshot: %v", err)
		return nil, status.Error(codes.Internal, "Failed to get snapshots")
	}

	var snap *obj2.Snapshot
	if len(snapshots) == 1 {
		snap := snapshots[0]

		if snap.VolumeID != volumeID {
			return nil, status.Error(codes.AlreadyExists, "Snapshot with given name already exists, with different source volume ID")
		}

		klog.V(3).Infof("Found existing snapshot %s from volume with ID: %s", name, volumeID)

	} else if len(snapshots) > 1 {
		klog.Errorf("found multiple existing snapshots with selected name (%s) during create", name)
		return nil, status.Error(codes.Internal, "Multiple snapshots reported by Cinder with same name")

	} else {
		snap, err = s.Cloud.CreateSnapshot(name, volumeID)
		if err != nil {
			klog.Errorf("Failed to Create snapshot: %v", err)
			return nil, status.Error(codes.Internal, fmt.Sprintf("CreateSnapshot failed with error %v", err))
		}

		klog.V(3).Infof("CreateSnapshot %s from volume with ID: %s", name, volumeID)
	}

	pt, err := parseTime(snap.CreatedAt)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to parse snapshot creation time [%v]; ERR: %v", snap.CreatedAt, err))
	}

	ctime := timestamppb.New(*pt)
	if err := ctime.CheckValid(); err != nil {
		klog.Errorf("Error to convert time to timestamp: %v", err)
	}

	err = s.Cloud.WaitSnapshotReady(snap.ID)
	if err != nil {
		klog.Errorf("Failed to WaitSnapshotReady: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("CreateSnapshot failed with error %v", err))
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     snap.ID,
			SizeBytes:      int64(snap.Size * 1024 * 1024 * 1024),
			SourceVolumeId: snap.VolumeID,
			CreationTime:   ctime,
			ReadyToUse:     true,
		},
	}, nil
}

func (s *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	klog.V(4).Infof("DeleteSnapshot: called with args %+v", protosanitizer.StripSecrets(*req))

	id := req.GetSnapshotId()
	snapshot, err := s.Cloud.GetSnapshotByID(id)
	if err != nil {
		klog.Errorf("DeleteSnapshot; Failed to get snapshot [ID=%s]: %v", id, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("DeleteSnapshot failed with error %v", err))
	}

	volumeID := snapshot.VolumeID

	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "Snapshot ID must be provided in DeleteSnapshot request")
	}

	// Delegate the check to openstack itself
	err = s.Cloud.DeleteSnapshot(volumeID, id)
	if err != nil {
		klog.Errorf("Failed to Delete snapshot: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("DeleteSnapshot failed with error %v", err))
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (s *controllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	klog.V(4).Infof("ListSnapshots; called with request %+v", req)
	snapshotID := req.GetSnapshotId()

	if snapshotID != "" {
		snap, err := s.Cloud.GetSnapshotByID(snapshotID)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get snapshot [%v]; ERR: %v", snapshotID, err))
		}

		pt, err := parseTime(snap.CreatedAt)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to parse snapshot creation time [%v]; ERR: %v", snap.CreatedAt, err))
		}

		ctime := timestamppb.New(*pt)
		entry := &csi.ListSnapshotsResponse_Entry{
			Snapshot: &csi.Snapshot{
				SizeBytes:      int64(snap.Size * 1024 * 1024 * 1024),
				SnapshotId:     snap.ID,
				SourceVolumeId: snap.VolumeID,
				CreationTime:   ctime,
				ReadyToUse:     true,
			},
		}

		entries := []*csi.ListSnapshotsResponse_Entry{entry}
		return &csi.ListSnapshotsResponse{
			Entries: entries,
		}, ctime.CheckValid()
	}

	var slist []*obj.Snapshot
	var err error
	var nextPageToken string

	slist, nextPageToken, err = s.Cloud.ListSnapshots(req.StartingToken, int(req.MaxEntries), req.GetSourceVolumeId(), "active", "")
	if err != nil {
		klog.Errorf("Failed to ListSnapshots: %v", err)
		return nil, status.Errorf(codes.Internal, "ListSnapshots failed with error %v", err)
	}

	sentries := make([]*csi.ListSnapshotsResponse_Entry, 0, len(slist))
	for _, v := range slist {
		ctime, err := parseTime(v.CreatedAt)
		if err != nil {
			klog.Errorf("Error to parse snapshot creation time [%v]; ERR: %v", v.CreatedAt, err)
		}
		sentry := csi.ListSnapshotsResponse_Entry{
			Snapshot: &csi.Snapshot{
				SizeBytes:      int64(v.Size * 1024 * 1024 * 1024),
				SnapshotId:     v.ID,
				SourceVolumeId: v.VolumeID,
				CreationTime:   &timestamp.Timestamp{Seconds: ctime.Unix()},
				ReadyToUse:     true,
			},
		}
		sentries = append(sentries, &sentry)
	}
	return &csi.ListSnapshotsResponse{
		Entries:   sentries,
		NextToken: nextPageToken,
	}, nil
}

func (s *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(5).Infof("ControllerGetCapabilities; called with request %+v", req)

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: s.Driver.cscap,
	}, nil
}

func (s *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) < 1 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities are required")
	}

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID is required")
	}

	_, err := s.Cloud.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get volume; ERR: %v", err))
	}

	for _, cap := range volCaps {
		if cap.GetAccessMode().GetMode() != s.Driver.vcap[0].Mode {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: "requested volume capability not supported"}, nil
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: s.Driver.vcap[0],
				},
			},
		},
	}, nil
}

func (s *controllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetCapacity is not yet implemented")
}

func (s *controllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	klog.V(4).Infof("ControllerGetVolume; called with request %+v", req)

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID is required")
	}

	volume, err := s.Cloud.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get volume; ERR: %v", err))
	}

	volEntry := csi.ControllerGetVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volume.VolumeId,
			CapacityBytes: int64(volume.Size * (1024 ^ 3))}}

	csiStatus := &csi.ControllerGetVolumeResponse_VolumeStatus{}
	if isAttachment(volume.VmId) {
		csiStatus.PublishedNodeIds = []string{*volume.VmId}
	}

	volEntry.Status = csiStatus

	return &volEntry, nil
}

func (s *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	klog.V(4).Infof("ControllerExpandVolume; called with request %+v", req)

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID is required")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range is required")
	}

	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	volSizeGB := uint64(utils.RoundUpSize(volSizeBytes, 1024*1024*1024))
	maxVolSize := capRange.GetLimitBytes()

	if maxVolSize > 0 && volSizeBytes > maxVolSize {
		return nil, status.Error(codes.OutOfRange, fmt.Sprintf("Requested size %d exceeds limit %d", volSizeBytes, maxVolSize))
	}

	volume, err := s.Cloud.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get volume; ERR: %v", err))
	}

	if volume == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("volume %s not found", volumeID))
	}

	if volume.Size >= volSizeGB {
		klog.V(2).Infof("ControllerExpandVolume; volume %s already has size %d GiB", volumeID, volume.Size)
		return &csi.ControllerExpandVolumeResponse{
			CapacityBytes:         int64(volume.Size * (1024 ^ 3)),
			NodeExpansionRequired: true,
		}, nil
	}

	err = s.Cloud.ExpandVolume(volume.VolumeTypeID, volumeID, volSizeGB)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to expand volume; ERR: %v", err))
	}

	err = s.Cloud.WaitVolumeTargetStatus(volumeID, []string{vcontainer.VolumeAvailableStatus, vcontainer.VolumeInUseStatus})
	if err != nil {
		klog.Errorf("ControllerExpandVolume; failed to wait volume %s to be available; ERR: %v", volumeID, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to wait volume to be available; ERR: %v", err))
	}

	klog.V(4).Infof("ControllerExpandVolume; volume %s expanded to size %d successfully", volumeID, volSizeGB)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         volSizeBytes,
		NodeExpansionRequired: true,
	}, nil
}

func parseTime(strTime string) (*time.Time, error) {
	parsedTime, err := time.Parse(layout, strTime)
	if err != nil {
		fmt.Println("Error:", err)
		return &parsedTime, fmt.Errorf("failed to parse time: %v", err)
	}

	return &parsedTime, nil
}
