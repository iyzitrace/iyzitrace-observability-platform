package detectors

import (
	"time"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// ServiceDetector detects service entities and their relationships to infrastructure.
type ServiceDetector struct{}

// NewServiceDetector creates a new service detector.
func NewServiceDetector() *ServiceDetector {
	return &ServiceDetector{}
}

// Name returns the detector name.
func (d *ServiceDetector) Name() string {
	return "service"
}

// Detect inspects attributes and emits service entities and relations.
func (d *ServiceDetector) Detect(item *model.TelemetryItem, keys adapter.SemconvKeys, enums adapter.SemconvEnums) *model.ClassificationResult {
	result := &model.ClassificationResult{
		Entities:  make([]*model.Entity, 0),
		Relations: make([]*model.Relation, 0),
	}

	attrs := item.Attrs
	now := time.Now()

	// Skip if no service name
	if !hasAttr(attrs, keys.ServiceName) {
		return result
	}

	serviceName := getAttr(attrs, keys.ServiceName)
	serviceNamespace := getAttr(attrs, keys.ServiceNamespace)
	serviceInstanceID := getAttr(attrs, keys.ServiceInstanceID)
	serviceVersion := getAttr(attrs, keys.ServiceVersion)
	deployEnv := getAttr(attrs, keys.DeploymentEnvironment)

	// Create display name
	displayName := serviceName
	if serviceNamespace != "" {
		displayName = serviceNamespace + "/" + serviceName
	}

	builder := model.NewEntityBuilder(model.EntityTypeService).
		WithName(displayName).
		WithAttr("service.name", serviceName).
		WithEvidence(createEvidence(item, "1.26.0", keys.ServiceName, serviceName))

	if serviceNamespace != "" {
		builder.WithAttr("service.namespace", serviceNamespace)
	}
	if serviceInstanceID != "" {
		builder.WithAttr("service.instance.id", serviceInstanceID)
	}
	if serviceVersion != "" {
		builder.WithAttr("service.version", serviceVersion)
	}
	if deployEnv != "" {
		builder.WithAttr("deployment.environment.name", deployEnv)
	}

	serviceEntity := builder.Build()
	result.Entities = append(result.Entities, serviceEntity)

	// Check for infrastructure relationships

	// Service runs_on host
	if hasAttr(attrs, keys.HostID) || hasAttr(attrs, keys.HostName) {
		hostID := getAttrAny(attrs, keys.HostID, keys.HostName)
		hostAttrs := map[string]string{}
		if h := getAttr(attrs, keys.HostID); h != "" {
			hostAttrs["host.id"] = h
		}
		if h := getAttr(attrs, keys.HostName); h != "" {
			hostAttrs["host.name"] = h
		}
		hostEntityID := model.GenerateEntityID(model.EntityTypeHost, hostAttrs)

		result.Relations = append(result.Relations, &model.Relation{
			FromID:    serviceEntity.ID,
			ToID:      hostEntityID,
			Type:      model.RelationTypeRunsOn,
			FirstSeen: now,
			LastSeen:  now,
			Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.ServiceName, serviceName)},
			Attrs:     map[string]string{"host.id": hostID},
		})
	}

	// Service runs_in container
	if hasAttr(attrs, keys.ContainerID) || hasAttr(attrs, keys.ContainerName) {
		containerID := getAttrAny(attrs, keys.ContainerID, keys.ContainerName)
		containerAttrs := map[string]string{}
		if c := getAttr(attrs, keys.ContainerID); c != "" {
			containerAttrs["container.id"] = c
		}
		if c := getAttr(attrs, keys.ContainerName); c != "" {
			containerAttrs["container.name"] = c
		}
		containerEntityID := model.GenerateEntityID(model.EntityTypeContainer, containerAttrs)

		result.Relations = append(result.Relations, &model.Relation{
			FromID:    serviceEntity.ID,
			ToID:      containerEntityID,
			Type:      model.RelationTypeRunsIn,
			FirstSeen: now,
			LastSeen:  now,
			Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.ServiceName, serviceName)},
			Attrs:     map[string]string{"container.id": containerID},
		})
	}

	// Service runs_in pod
	if hasAttr(attrs, keys.K8sPodName) {
		podName := getAttr(attrs, keys.K8sPodName)
		podAttrs := map[string]string{
			"k8s.pod.name": podName,
		}
		if ns := getAttr(attrs, keys.K8sNamespaceName); ns != "" {
			podAttrs["k8s.namespace.name"] = ns
		}
		if cluster := getAttr(attrs, keys.K8sClusterName); cluster != "" {
			podAttrs["k8s.cluster.name"] = cluster
		}
		podEntityID := model.GenerateEntityID(model.EntityTypeK8sPod, podAttrs)

		result.Relations = append(result.Relations, &model.Relation{
			FromID:    serviceEntity.ID,
			ToID:      podEntityID,
			Type:      model.RelationTypeRunsIn,
			FirstSeen: now,
			LastSeen:  now,
			Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.ServiceName, serviceName)},
			Attrs:     map[string]string{"k8s.pod.name": podName},
		})
	}



	return result
}
