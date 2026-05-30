// Package model defines the data model for the Inventory Service.
// It contains Entity, Relation, and Evidence types used throughout the system.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// EntityType represents the type of an entity in the inventory.
type EntityType string

const (
	// Infrastructure types
	EntityTypeCloudRegion   EntityType = "cloud.region"
	EntityTypeHost          EntityType = "host"
	EntityTypeContainer     EntityType = "container"
	EntityTypeProcess       EntityType = "process"

	// Kubernetes types
	EntityTypeK8sCluster    EntityType = "k8s.cluster"
	EntityTypeK8sNode       EntityType = "k8s.node"
	EntityTypeK8sNamespace  EntityType = "k8s.namespace"
	EntityTypeK8sDeployment EntityType = "k8s.deployment"
	EntityTypeK8sReplicaSet EntityType = "k8s.replicaset"
	EntityTypeK8sStatefulSet EntityType = "k8s.statefulset"
	EntityTypeK8sDaemonSet  EntityType = "k8s.daemonset"
	EntityTypeK8sPod        EntityType = "k8s.pod"

	// Service types
	EntityTypeService EntityType = "service"

	// Database types
	EntityTypeDBInstance EntityType = "db.instance"
	EntityTypeDBDatabase EntityType = "db.database"

	// Cache types
	EntityTypeCacheInstance EntityType = "cache.instance"

	// Messaging types
	EntityTypeMessagingSystem      EntityType = "messaging.system"
	EntityTypeMessagingDestination EntityType = "messaging.destination"
	EntityTypeMessagingProducer    EntityType = "messaging.producer"
	EntityTypeMessagingConsumer    EntityType = "messaging.consumer"

	// Mobile types
	EntityTypeMobileAppIOS     EntityType = "mobile.app.ios"
	EntityTypeMobileAppAndroid EntityType = "mobile.app.android"
	EntityTypeMobileApp        EntityType = "mobile.app"
)

// EntityStatus represents the lifecycle status of an entity.
type EntityStatus string

const (
	EntityStatusActive  EntityStatus = "active"  // Recently seen
	EntityStatusStale   EntityStatus = "stale"   // Not seen for a while but might come back
	EntityStatusStopped EntityStatus = "stopped" // Confirmed terminated/deleted
)

// InstanceInfo tracks a specific instance of a logical entity.
// For example, a container might be recreated multiple times with different IDs.
type InstanceInfo struct {
	InstanceID string    `json:"instance_id"`          // Unique ID for this instance (e.g., container.id, pod UID)
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	Status     EntityStatus `json:"status"`
	Attrs      map[string]string `json:"attrs,omitempty"` // Instance-specific attributes
}

// RelationType represents the type of relationship between entities.
type RelationType string

const (
	RelationTypeRunsOn       RelationType = "runs_on"        // service → host
	RelationTypeRunsIn       RelationType = "runs_in"        // service → container/pod
	RelationTypeLocatedIn    RelationType = "located_in"     // host → cloud.region
	RelationTypeUses         RelationType = "uses"           // service → db.instance/cache.instance/messaging.system
	RelationTypePublishesTo  RelationType = "publishes_to"   // producer/service → messaging.destination
	RelationTypeConsumesFrom RelationType = "consumes_from"  // consumer/service → messaging.destination
	RelationTypeBelongsTo    RelationType = "belongs_to"     // db.database → db.instance
	RelationTypePartOf       RelationType = "part_of"        // k8s.pod → k8s.namespace/cluster
	RelationTypeManagedBy    RelationType = "managed_by"     // k8s.pod → k8s.deployment/replicaset
	RelationTypeInstanceOf   RelationType = "instance_of"    // container instance → container logical
)

// SignalType represents the type of telemetry signal.
type SignalType string

const (
	SignalTypeResource SignalType = "resource"
	SignalTypeSpan     SignalType = "span"
	SignalTypeLog      SignalType = "log"
	SignalTypeMetric   SignalType = "metric"
)

// SignalSource holds identifiers for the source signal (trace_id, span_id, metric_name, etc.).
// This serves as "proof" of where an entity/relation was discovered.
type SignalSource struct {
	// Common fields
	SignalType SignalType `json:"signal_type"`
	Timestamp  time.Time  `json:"timestamp"`

	// Trace-specific fields
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
	SpanName string `json:"span_name,omitempty"`

	// Metric-specific fields
	MetricName string `json:"metric_name,omitempty"`
	MetricType string `json:"metric_type,omitempty"`

	// Log-specific fields
	LogSeverity string `json:"log_severity,omitempty"`
	LogBody     string `json:"log_body,omitempty"` // First 100 chars for reference

	// Resource-specific fields (common across all)
	ServiceName      string `json:"service_name,omitempty"`
	ServiceNamespace string `json:"service_namespace,omitempty"`
}

// Evidence records why an entity or relation was inferred.
type Evidence struct {
	SignalType     SignalType `json:"signal_type"`
	AttributeKey   string     `json:"attribute_key"`
	AttributeValue string     `json:"attribute_value"`
	SemconvVersion string     `json:"semconv_version"`
	Timestamp      time.Time  `json:"timestamp"`

	// Source information - proof of where this was discovered
	Source *SignalSource `json:"source,omitempty"`
}

// Entity represents a discovered entity in the inventory.
type Entity struct {
	ID        string            `json:"id"`
	Type      EntityType        `json:"type"`
	Name      string            `json:"name"`
	Attrs     map[string]string `json:"attrs"`
	FirstSeen time.Time         `json:"first_seen"`
	LastSeen  time.Time         `json:"last_seen"`
	Evidence  []Evidence        `json:"evidence"`

	// Lifecycle tracking
	Status        EntityStatus   `json:"status"`
	InstanceCount int            `json:"instance_count"`   // Total instances seen
	ActiveCount   int            `json:"active_count"`     // Currently active instances
	Instances     []InstanceInfo `json:"instances,omitempty"` // Recent instance history (last N)
}

// Relation represents a relationship between two entities.
type Relation struct {
	FromID    string            `json:"from_id"`
	ToID      string            `json:"to_id"`
	Type      RelationType      `json:"type"`
	Attrs     map[string]string `json:"attrs,omitempty"`
	FirstSeen time.Time         `json:"first_seen"`
	LastSeen  time.Time         `json:"last_seen"`
	Evidence  []Evidence        `json:"evidence"`
}

// EntityBuilder helps construct Entity objects with proper ID generation.
type EntityBuilder struct {
	entityType EntityType
	name       string
	attrs      map[string]string
	evidence   []Evidence
	instances  []InstanceInfo
}

// NewEntityBuilder creates a new EntityBuilder.
func NewEntityBuilder(entityType EntityType) *EntityBuilder {
	return &EntityBuilder{
		entityType: entityType,
		attrs:      make(map[string]string),
		evidence:   make([]Evidence, 0),
		instances:  make([]InstanceInfo, 0),
	}
}

// WithName sets the entity name.
func (b *EntityBuilder) WithName(name string) *EntityBuilder {
	b.name = name
	return b
}

// WithAttr adds an attribute to the entity.
func (b *EntityBuilder) WithAttr(key, value string) *EntityBuilder {
	b.attrs[key] = value
	return b
}

// WithAttrs adds multiple attributes to the entity.
func (b *EntityBuilder) WithAttrs(attrs map[string]string) *EntityBuilder {
	for k, v := range attrs {
		b.attrs[k] = v
	}
	return b
}

// WithEvidence adds evidence to the entity.
func (b *EntityBuilder) WithEvidence(evidence Evidence) *EntityBuilder {
	b.evidence = append(b.evidence, evidence)
	return b
}

// WithInstance adds an instance to the entity.
func (b *EntityBuilder) WithInstance(instanceID string, attrs map[string]string) *EntityBuilder {
	now := time.Now()
	b.instances = append(b.instances, InstanceInfo{
		InstanceID: instanceID,
		FirstSeen:  now,
		LastSeen:   now,
		Status:     EntityStatusActive,
		Attrs:      attrs,
	})
	return b
}

// Build constructs the Entity with a generated ID.
func (b *EntityBuilder) Build() *Entity {
	now := time.Now()
	id := GenerateEntityID(b.entityType, b.attrs)
	return &Entity{
		ID:            id,
		Type:          b.entityType,
		Name:          b.name,
		Attrs:         b.attrs,
		FirstSeen:     now,
		LastSeen:      now,
		Evidence:      b.evidence,
		Status:        EntityStatusActive,
		InstanceCount: len(b.instances),
		ActiveCount:   len(b.instances),
		Instances:     b.instances,
	}
}

// GenerateEntityID creates a deterministic ID for an entity based on its type and key attributes.
func GenerateEntityID(entityType EntityType, attrs map[string]string) string {
	var parts []string
	parts = append(parts, string(entityType))

	switch entityType {
	case EntityTypeHost:
		// Prefer host.name as it is more common across signals (logs/metrics vs traces).
		// Fallback to host.id if name is missing.
		if name, ok := attrs["host.name"]; ok && name != "" {
			parts = append(parts, normalizeAttr(name))
		} else if id, ok := attrs["host.id"]; ok && id != "" {
			parts = append(parts, normalizeAttr(id))
		}

	case EntityTypeService:
		// service.namespace + "/" + service.name
		namespace := normalizeAttr(attrs["service.namespace"])
		name := normalizeAttr(attrs["service.name"])
		if namespace != "" {
			parts = append(parts, namespace+"/"+name)
		} else {
			parts = append(parts, name)
		}

	case EntityTypeDBInstance, EntityTypeCacheInstance:
		// db.system + "|" + server.address + "|" + server.port
		system := normalizeAttr(attrs["db.system"])
		addr := normalizeAttr(getFirstNonEmpty(attrs, "server.address", "net.peer.name", "peer.service"))
		port := normalizeAttr(getFirstNonEmpty(attrs, "server.port", "net.peer.port"))
		parts = append(parts, fmt.Sprintf("%s|%s|%s", system, addr, port))

	case EntityTypeDBDatabase:
		// db.instance_id + "/" + db.name
		instanceID := normalizeAttr(attrs["db.instance.id"])
		dbName := normalizeAttr(attrs["db.name"])
		parts = append(parts, fmt.Sprintf("%s/%s", instanceID, dbName))

	case EntityTypeMessagingSystem:
		// messaging.system + "|" + server.address + "|" + server.port
		system := normalizeAttr(attrs["messaging.system"])
		addr := normalizeAttr(getFirstNonEmpty(attrs, "server.address", "net.peer.name"))
		port := normalizeAttr(getFirstNonEmpty(attrs, "server.port", "net.peer.port"))
		parts = append(parts, fmt.Sprintf("%s|%s|%s", system, addr, port))

	case EntityTypeMessagingDestination:
		// messaging.system_id + "/" + messaging.destination.name
		systemID := normalizeAttr(attrs["messaging.system.id"])
		destName := normalizeAttr(attrs["messaging.destination.name"])
		destKind := normalizeAttr(attrs["messaging.destination.kind"])
		if destKind != "" {
			parts = append(parts, fmt.Sprintf("%s/%s/%s", systemID, destKind, destName))
		} else {
			parts = append(parts, fmt.Sprintf("%s/%s", systemID, destName))
		}

	case EntityTypeCloudRegion:
		// cloud.provider + "/" + cloud.region
		provider := normalizeAttr(attrs["cloud.provider"])
		region := normalizeAttr(attrs["cloud.region"])
		if provider != "" {
			parts = append(parts, fmt.Sprintf("%s/%s", provider, region))
		} else {
			parts = append(parts, region)
		}

	case EntityTypeContainer:
		// Container ID is gold standard.
		if id, ok := attrs["container.id"]; ok && id != "" {
			parts = append(parts, normalizeAttr(id))
		} else {
			// Fallback: name + image + host
			containerName := normalizeAttr(attrs["container.name"])
			imageName := normalizeAttr(attrs["container.image.name"])
			hostID := normalizeAttr(getFirstNonEmpty(attrs, "host.id", "host.name"))
			
			if containerName != "" && imageName != "" {
				parts = append(parts, fmt.Sprintf("%s|%s|%s", hostID, containerName, imageName))
			} else if containerName != "" {
				parts = append(parts, fmt.Sprintf("%s|%s", hostID, containerName))
			}
		}

	case EntityTypeProcess:
		// host + pid combination
		host := normalizeAttr(getFirstNonEmpty(attrs, "host.id", "host.name"))
		pid := normalizeAttr(attrs["process.pid"])
		parts = append(parts, fmt.Sprintf("%s|%s", host, pid))

	case EntityTypeK8sCluster:
		parts = append(parts, normalizeAttr(attrs["k8s.cluster.name"]))

	case EntityTypeK8sNamespace:
		cluster := normalizeAttr(attrs["k8s.cluster.name"])
		ns := normalizeAttr(attrs["k8s.namespace.name"])
		parts = append(parts, fmt.Sprintf("%s/%s", cluster, ns))

	case EntityTypeK8sPod:
		cluster := normalizeAttr(attrs["k8s.cluster.name"])
		ns := normalizeAttr(attrs["k8s.namespace.name"])
		pod := normalizeAttr(attrs["k8s.pod.name"])
		// Prefer pod UID if available
		if podUID := normalizeAttr(attrs["k8s.pod.uid"]); podUID != "" {
			parts = append(parts, podUID)
		} else {
			parts = append(parts, fmt.Sprintf("%s/%s/%s", cluster, ns, pod))
		}

	case EntityTypeK8sDeployment:
		cluster := normalizeAttr(attrs["k8s.cluster.name"])
		ns := normalizeAttr(attrs["k8s.namespace.name"])
		deployment := normalizeAttr(attrs["k8s.deployment.name"])
		parts = append(parts, fmt.Sprintf("%s/%s/%s", cluster, ns, deployment))

	case EntityTypeK8sReplicaSet:
		cluster := normalizeAttr(attrs["k8s.cluster.name"])
		ns := normalizeAttr(attrs["k8s.namespace.name"])
		rs := normalizeAttr(attrs["k8s.replicaset.name"])
		parts = append(parts, fmt.Sprintf("%s/%s/%s", cluster, ns, rs))

	case EntityTypeK8sStatefulSet:
		cluster := normalizeAttr(attrs["k8s.cluster.name"])
		ns := normalizeAttr(attrs["k8s.namespace.name"])
		sts := normalizeAttr(attrs["k8s.statefulset.name"])
		parts = append(parts, fmt.Sprintf("%s/%s/%s", cluster, ns, sts))

	case EntityTypeK8sDaemonSet:
		cluster := normalizeAttr(attrs["k8s.cluster.name"])
		ns := normalizeAttr(attrs["k8s.namespace.name"])
		ds := normalizeAttr(attrs["k8s.daemonset.name"])
		parts = append(parts, fmt.Sprintf("%s/%s/%s", cluster, ns, ds))

	case EntityTypeK8sNode:
		cluster := normalizeAttr(attrs["k8s.cluster.name"])
		node := normalizeAttr(attrs["k8s.node.name"])
		parts = append(parts, fmt.Sprintf("%s/%s", cluster, node))

	case EntityTypeMobileApp, EntityTypeMobileAppIOS, EntityTypeMobileAppAndroid:
		name := normalizeAttr(attrs["service.name"])
		platform := normalizeAttr(attrs["os.type"])
		parts = append(parts, fmt.Sprintf("%s|%s", name, platform))

	default:
		// For unknown types, use all attributes
		for k, v := range attrs {
			parts = append(parts, fmt.Sprintf("%s=%s", k, normalizeAttr(v)))
		}
	}

	// Generate a hash-based ID
	combined := strings.Join(parts, "::")
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes for shorter IDs
}

// GenerateRelationID creates a deterministic ID for a relation.
func GenerateRelationID(fromID, toID string, relType RelationType) string {
	combined := fmt.Sprintf("%s::%s::%s", fromID, toID, relType)
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:16])
}

// getFirstNonEmpty returns the first non-empty value from the given attribute keys.
func getFirstNonEmpty(attrs map[string]string, keys ...string) string {
	for _, key := range keys {
		if val, ok := attrs[key]; ok && val != "" {
			return val
		}
	}
	return ""
}

// normalizeAttr lowercases and trims whitespace from a string.
func normalizeAttr(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizedAttrs is a type alias for normalized attribute map.
type NormalizedAttrs map[string]string

// TelemetryItem represents a single telemetry item (span/log/metric) for processing.
type TelemetryItem struct {
	SignalType SignalType
	Attrs      NormalizedAttrs // Merged attributes (record > scope > resource)
	Timestamp  time.Time
	Source     *SignalSource   // Source identification for provenance tracking
}

// ClassificationResult holds the results of classifying a telemetry item.
type ClassificationResult struct {
	Entities  []*Entity
	Relations []*Relation
}
