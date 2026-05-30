package detectors

import (
	"time"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// InfraDetector detects infrastructure entities (cloud, host, container, process).
type InfraDetector struct{}

// NewInfraDetector creates a new infrastructure detector.
func NewInfraDetector() *InfraDetector {
	return &InfraDetector{}
}

// Name returns the detector name.
func (d *InfraDetector) Name() string {
	return "infra"
}

// Detect inspects attributes and emits infrastructure entities and relations.
func (d *InfraDetector) Detect(item *model.TelemetryItem, keys adapter.SemconvKeys, enums adapter.SemconvEnums) *model.ClassificationResult {
	result := &model.ClassificationResult{
		Entities:  make([]*model.Entity, 0),
		Relations: make([]*model.Relation, 0),
	}

	attrs := item.Attrs
	now := time.Now()

	// Detect Cloud Region
	var regionEntity *model.Entity
	if hasAttr(attrs, keys.CloudRegion) {
		region := getAttr(attrs, keys.CloudRegion)
		provider := getAttr(attrs, keys.CloudProvider)
		az := getAttr(attrs, keys.CloudAvailabilityZone)

		builder := model.NewEntityBuilder(model.EntityTypeCloudRegion).
			WithName(region).
			WithAttr("cloud.region", region).
			WithEvidence(createEvidence(item, "1.26.0", keys.CloudRegion, region))

		if provider != "" {
			builder.WithAttr("cloud.provider", provider)
		}
		if az != "" {
			builder.WithAttr("cloud.availability_zone", az)
		}

		regionEntity = builder.Build()
		result.Entities = append(result.Entities, regionEntity)
	}

	// Detect Host
	var hostEntity *model.Entity
	if hasAttr(attrs, keys.HostID) || hasAttr(attrs, keys.HostName) {
		hostID := getAttr(attrs, keys.HostID)
		hostName := getAttr(attrs, keys.HostName)
		hostType := getAttr(attrs, keys.HostType)
		hostArch := getAttr(attrs, keys.HostArch)
		osType := getAttr(attrs, keys.OSType)

		name := hostName
		if name == "" {
			name = hostID
		}

		builder := model.NewEntityBuilder(model.EntityTypeHost).
			WithName(name)

		evidenceKey := keys.HostName
		evidenceValue := hostName
		if hostID != "" {
			builder.WithAttr("host.id", hostID)
			evidenceKey = keys.HostID
			evidenceValue = hostID
		}
		if hostName != "" {
			builder.WithAttr("host.name", hostName)
		}
		if hostType != "" {
			builder.WithAttr("host.type", hostType)
		}
		if hostArch != "" {
			builder.WithAttr("host.arch", hostArch)
		}
		if osType != "" {
			builder.WithAttr("os.type", osType)
		}

		builder.WithEvidence(createEvidence(item, "1.26.0", evidenceKey, evidenceValue))
		hostEntity = builder.Build()
		result.Entities = append(result.Entities, hostEntity)

		// If no region exists, create a default region
		if regionEntity == nil {
			regionEntity = model.NewEntityBuilder(model.EntityTypeCloudRegion).
				WithName("default-region").
				WithAttr("cloud.region", "default-region").
				WithAttr("cloud.provider", "onprem").
				WithEvidence(createEvidence(item, "1.26.0", "cloud.region", "default-region")).
				Build()
			result.Entities = append(result.Entities, regionEntity)
		}

		// Relation: host located_in region
		result.Relations = append(result.Relations, &model.Relation{
			FromID:    hostEntity.ID,
			ToID:      regionEntity.ID,
			Type:      model.RelationTypeLocatedIn,
			FirstSeen: now,
			LastSeen:  now,
			Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.CloudRegion, getAttr(attrs, keys.CloudRegion))},
		})
	}

	// Detect Container
	var containerEntity *model.Entity
	if hasAttr(attrs, keys.ContainerID) || hasAttr(attrs, keys.ContainerName) {
		containerID := getAttr(attrs, keys.ContainerID)
		containerName := getAttr(attrs, keys.ContainerName)
		containerRuntime := getAttr(attrs, keys.ContainerRuntime)
		containerImageName := getAttr(attrs, keys.ContainerImageName)
		containerImageID := getAttr(attrs, keys.ContainerImageID)
		hostID := getAttrAny(attrs, keys.HostID, keys.HostName)

		name := containerName
		if name == "" {
			name = containerID
		}

		builder := model.NewEntityBuilder(model.EntityTypeContainer).
			WithName(name)

		evidenceKey := keys.ContainerName
		evidenceValue := containerName
		if containerID != "" {
			// Track container.id as an instance, not the entity ID
			builder.WithInstance(containerID, map[string]string{
				"container.id":      containerID,
				"container.runtime": containerRuntime,
			})
			evidenceKey = keys.ContainerID
			evidenceValue = containerID
		}
		if containerName != "" {
			builder.WithAttr("container.name", containerName)
		}
		if containerRuntime != "" {
			builder.WithAttr("container.runtime", containerRuntime)
		}
		if containerImageName != "" {
			builder.WithAttr("container.image.name", containerImageName)
		}
		if containerImageID != "" {
			builder.WithAttr("container.image.id", containerImageID)
		}
		if hostID != "" {
			builder.WithAttr("host.id", hostID)
		}

		builder.WithEvidence(createEvidence(item, "1.26.0", evidenceKey, evidenceValue))
		containerEntity = builder.Build()
		result.Entities = append(result.Entities, containerEntity)

		// Relation: container runs_on host
		if hostEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    containerEntity.ID,
				ToID:      hostEntity.ID,
				Type:      model.RelationTypeRunsOn,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", evidenceKey, evidenceValue)},
			})
		}
	}

	// Detect Process
	if hasAttr(attrs, keys.ProcessPID) {
		pid := getAttr(attrs, keys.ProcessPID)
		execName := getAttr(attrs, keys.ProcessExecutableName)
		execPath := getAttr(attrs, keys.ProcessExecutablePath)
		command := getAttr(attrs, keys.ProcessCommand)
		runtimeName := getAttr(attrs, keys.ProcessRuntimeName)
		runtimeVersion := getAttr(attrs, keys.ProcessRuntimeVersion)

		// Use host info for unique identification
		hostID := getAttrAny(attrs, keys.HostID, keys.HostName)

		name := execName
		if name == "" {
			name = command
		}
		if name == "" {
			name = "process-" + pid
		}

		builder := model.NewEntityBuilder(model.EntityTypeProcess).
			WithName(name).
			WithAttr("process.pid", pid).
			WithEvidence(createEvidence(item, "1.26.0", keys.ProcessPID, pid))

		if hostID != "" {
			builder.WithAttr("host.id", hostID)
		}
		if execName != "" {
			builder.WithAttr("process.executable.name", execName)
		}
		if execPath != "" {
			builder.WithAttr("process.executable.path", execPath)
		}
		if command != "" {
			builder.WithAttr("process.command", command)
		}
		if runtimeName != "" {
			builder.WithAttr("process.runtime.name", runtimeName)
		}
		if runtimeVersion != "" {
			builder.WithAttr("process.runtime.version", runtimeVersion)
		}

		processEntity := builder.Build()
		result.Entities = append(result.Entities, processEntity)

		// Relation: process runs_on host or container
		if containerEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    processEntity.ID,
				ToID:      containerEntity.ID,
				Type:      model.RelationTypeRunsIn,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.ProcessPID, pid)},
			})
		} else if hostEntity != nil {
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    processEntity.ID,
				ToID:      hostEntity.ID,
				Type:      model.RelationTypeRunsOn,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.ProcessPID, pid)},
			})
		}
	}

	return result
}
