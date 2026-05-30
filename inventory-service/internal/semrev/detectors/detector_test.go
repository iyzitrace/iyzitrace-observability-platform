package detectors

import (
	"testing"
	"time"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

func TestInfraDetector(t *testing.T) {
	detector := NewInfraDetector()
	adp := adapter.NewV1260Adapter()
	keys := adp.Keys()
	enums := adp.Enums()

	tests := []struct {
		name           string
		attrs          model.NormalizedAttrs
		expectedTypes  []model.EntityType
		expectedRels   []model.RelationType
	}{
		{
			name: "detects host from host.name",
			attrs: model.NormalizedAttrs{
				"host.name": "my-server",
				"host.type": "virtual",
				"os.type":   "linux",
			},
			expectedTypes: []model.EntityType{model.EntityTypeHost, model.EntityTypeCloudRegion},
			expectedRels:  []model.RelationType{model.RelationTypeLocatedIn},
		},
		{
			name: "detects host and region with relation",
			attrs: model.NormalizedAttrs{
				"host.name":      "my-server",
				"cloud.region":   "us-east-1",
				"cloud.provider": "aws",
			},
			expectedTypes: []model.EntityType{model.EntityTypeCloudRegion, model.EntityTypeHost},
			expectedRels:  []model.RelationType{model.RelationTypeLocatedIn},
		},
		{
			name: "detects container",
			attrs: model.NormalizedAttrs{
				"container.id":      "abc123",
				"container.name":    "my-container",
				"container.runtime": "docker",
			},
			expectedTypes: []model.EntityType{model.EntityTypeContainer},
			expectedRels:  []model.RelationType{},
		},
		{
			name: "detects container and host with relation",
			attrs: model.NormalizedAttrs{
				"host.name":         "my-server",
				"container.id":      "abc123",
				"container.name":    "my-container",
			},
			expectedTypes: []model.EntityType{model.EntityTypeHost, model.EntityTypeContainer, model.EntityTypeCloudRegion},
			expectedRels:  []model.RelationType{model.RelationTypeRunsOn, model.RelationTypeLocatedIn},
		},
		{
			name: "detects process",
			attrs: model.NormalizedAttrs{
				"process.pid":             "1234",
				"process.executable.name": "java",
				"host.name":               "my-server",
			},
			expectedTypes: []model.EntityType{model.EntityTypeHost, model.EntityTypeProcess, model.EntityTypeCloudRegion},
			expectedRels:  []model.RelationType{model.RelationTypeRunsOn, model.RelationTypeLocatedIn},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &model.TelemetryItem{
				SignalType: model.SignalTypeSpan,
				Attrs:      tt.attrs,
				Timestamp:  time.Now(),
			}

			result := detector.Detect(item, keys, enums)

			// Check entity types
			if len(result.Entities) != len(tt.expectedTypes) {
				t.Errorf("expected %d entities, got %d", len(tt.expectedTypes), len(result.Entities))
			}

			for _, expectedType := range tt.expectedTypes {
				found := false
				for _, entity := range result.Entities {
					if entity.Type == expectedType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected entity type %s not found", expectedType)
				}
			}

			// Check relation types
			if len(result.Relations) != len(tt.expectedRels) {
				t.Errorf("expected %d relations, got %d", len(tt.expectedRels), len(result.Relations))
			}

			for _, expectedType := range tt.expectedRels {
				found := false
				for _, rel := range result.Relations {
					if rel.Type == expectedType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected relation type %s not found", expectedType)
				}
			}
		})
	}
}

func TestServiceDetector(t *testing.T) {
	detector := NewServiceDetector()
	adp := adapter.NewV1260Adapter()
	keys := adp.Keys()
	enums := adp.Enums()

	tests := []struct {
		name          string
		attrs         model.NormalizedAttrs
		expectedName  string
		expectedRels  int
		expectedEnts  int
	}{
		{
			name: "detects service with namespace",
			attrs: model.NormalizedAttrs{
				"service.name":      "payment-service",
				"service.namespace": "prod",
				"service.version":   "1.0.0",
			},
			expectedName: "prod/payment-service",
			expectedRels: 0,
			expectedEnts: 1,
		},
		{
			name: "detects service without namespace",
			attrs: model.NormalizedAttrs{
				"service.name": "api-gateway",
			},
			expectedName: "api-gateway",
			expectedRels: 0,
			expectedEnts: 1,
		},
		{
			name: "detects service with host relation",
			attrs: model.NormalizedAttrs{
				"service.name": "user-service",
				"host.name":    "server-1",
			},
			expectedName: "user-service",
			expectedRels: 1,
			expectedEnts: 1,
		},
		{
			name: "detects service with pod relation",
			attrs: model.NormalizedAttrs{
				"service.name":       "order-service",
				"k8s.pod.name":       "order-service-xyz",
				"k8s.namespace.name": "default",
			},
			expectedName: "order-service",
			expectedRels: 1,
			expectedEnts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &model.TelemetryItem{
				SignalType: model.SignalTypeSpan,
				Attrs:      tt.attrs,
				Timestamp:  time.Now(),
			}

			result := detector.Detect(item, keys, enums)

			if len(result.Entities) != tt.expectedEnts {
				t.Fatalf("expected %d entity, got %d", tt.expectedEnts, len(result.Entities))
			}

			var entity *model.Entity
			for _, e := range result.Entities {
				if e.Type == model.EntityTypeService {
					entity = e
					break
				}
			}
			if entity == nil {
				t.Fatalf("expected service entity, got none")
			}

			if entity.Name != tt.expectedName {
				t.Errorf("expected name %s, got %s", tt.expectedName, entity.Name)
			}

			if len(result.Relations) != tt.expectedRels {
				t.Errorf("expected %d relations, got %d", tt.expectedRels, len(result.Relations))
			}
		})
	}
}

func TestDatabaseDetector(t *testing.T) {
	detector := NewDatabaseDetector()
	adp := adapter.NewV1260Adapter()
	keys := adp.Keys()
	enums := adp.Enums()

	tests := []struct {
		name         string
		attrs        model.NormalizedAttrs
		expectedType model.EntityType
		hasDBEntity  bool
		hasRelation  bool
	}{
		{
			name: "detects postgresql database",
			attrs: model.NormalizedAttrs{
				"db.system":      "postgresql",
				"db.name":        "users",
				"server.address": "db.example.com",
				"server.port":    "5432",
			},
			expectedType: model.EntityTypeDBInstance,
			hasDBEntity:  true,
			hasRelation:  true, // db.database belongs_to db.instance
		},
		{
			name: "detects redis as cache",
			attrs: model.NormalizedAttrs{
				"db.system":      "redis",
				"server.address": "redis.example.com",
				"server.port":    "6379",
			},
			expectedType: model.EntityTypeCacheInstance,
			hasDBEntity:  false,
			hasRelation:  false,
		},
		{
			name: "detects database with service relation",
			attrs: model.NormalizedAttrs{
				"db.system":    "mysql",
				"db.name":      "orders",
				"service.name": "order-service",
			},
			expectedType: model.EntityTypeDBInstance,
			hasDBEntity:  true,
			hasRelation:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &model.TelemetryItem{
				SignalType: model.SignalTypeSpan,
				Attrs:      tt.attrs,
				Timestamp:  time.Now(),
			}

			result := detector.Detect(item, keys, enums)

			if len(result.Entities) == 0 {
				t.Fatal("expected at least 1 entity")
			}

			// Check first entity is the expected type
			if result.Entities[0].Type != tt.expectedType {
				t.Errorf("expected entity type %s, got %s", tt.expectedType, result.Entities[0].Type)
			}

			// Check for db.database entity
			hasDB := false
			for _, e := range result.Entities {
				if e.Type == model.EntityTypeDBDatabase {
					hasDB = true
					break
				}
			}
			if hasDB != tt.hasDBEntity {
				t.Errorf("expected hasDBEntity=%v, got %v", tt.hasDBEntity, hasDB)
			}
		})
	}
}

func TestMessagingDetector(t *testing.T) {
	detector := NewMessagingDetector()
	adp := adapter.NewV1260Adapter()
	keys := adp.Keys()
	enums := adp.Enums()

	tests := []struct {
		name            string
		attrs           model.NormalizedAttrs
		expectedSystem  string
		hasDest         bool
		expectedRelType model.RelationType
	}{
		{
			name: "detects kafka producer",
			attrs: model.NormalizedAttrs{
				"messaging.system":           "kafka",
				"messaging.destination.name": "orders",
				"messaging.destination.kind": "topic",
				"messaging.operation":        "publish",
				"service.name":               "order-producer",
			},
			expectedSystem:  "kafka",
			hasDest:         true,
			expectedRelType: model.RelationTypePublishesTo,
		},
		{
			name: "detects kafka consumer",
			attrs: model.NormalizedAttrs{
				"messaging.system":           "kafka",
				"messaging.destination.name": "orders",
				"messaging.destination.kind": "topic",
				"messaging.operation":        "receive",
				"service.name":               "order-consumer",
			},
			expectedSystem:  "kafka",
			hasDest:         true,
			expectedRelType: model.RelationTypeConsumesFrom,
		},
		{
			name: "detects rabbitmq queue",
			attrs: model.NormalizedAttrs{
				"messaging.system":           "rabbitmq",
				"messaging.destination.name": "tasks",
				"messaging.destination.kind": "queue",
			},
			expectedSystem: "rabbitmq",
			hasDest:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &model.TelemetryItem{
				SignalType: model.SignalTypeSpan,
				Attrs:      tt.attrs,
				Timestamp:  time.Now(),
			}

			result := detector.Detect(item, keys, enums)

			if len(result.Entities) == 0 {
				t.Fatal("expected at least 1 entity")
			}

			// Check for messaging system entity
			var systemEntity *model.Entity
			for _, e := range result.Entities {
				if e.Type == model.EntityTypeMessagingSystem {
					systemEntity = e
					break
				}
			}
			if systemEntity == nil {
				t.Fatal("expected messaging.system entity")
			}

			// Check for destination entity
			var hasDest bool
			for _, e := range result.Entities {
				if e.Type == model.EntityTypeMessagingDestination {
					hasDest = true
					break
				}
			}
			if hasDest != tt.hasDest {
				t.Errorf("expected hasDest=%v, got %v", tt.hasDest, hasDest)
			}

			// Check for expected relation type
			if tt.expectedRelType != "" {
				found := false
				for _, rel := range result.Relations {
					if rel.Type == tt.expectedRelType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected relation type %s not found", tt.expectedRelType)
				}
			}
		})
	}
}

func TestK8sDetector(t *testing.T) {
	detector := NewK8sDetector()
	adp := adapter.NewV1260Adapter()
	keys := adp.Keys()
	enums := adp.Enums()

	tests := []struct {
		name           string
		attrs          model.NormalizedAttrs
		expectedTypes  []model.EntityType
		expectedRels   int
	}{
		{
			name: "detects full k8s hierarchy",
			attrs: model.NormalizedAttrs{
				"k8s.cluster.name":   "prod-cluster",
				"k8s.namespace.name": "default",
				"k8s.pod.name":       "my-app-xyz",
				"k8s.node.name":      "node-1",
			},
			expectedTypes: []model.EntityType{
				model.EntityTypeK8sCluster,
				model.EntityTypeK8sNamespace,
				model.EntityTypeK8sNode,
				model.EntityTypeK8sPod,
			},
			expectedRels: 4, // namespace part_of cluster, node part_of cluster, pod part_of namespace, pod runs_on node
		},
		{
			name: "detects only namespace and pod",
			attrs: model.NormalizedAttrs{
				"k8s.namespace.name": "staging",
				"k8s.pod.name":       "api-server-123",
			},
			expectedTypes: []model.EntityType{
				model.EntityTypeK8sNamespace,
				model.EntityTypeK8sPod,
			},
			expectedRels: 1, // pod part_of namespace
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &model.TelemetryItem{
				SignalType: model.SignalTypeSpan,
				Attrs:      tt.attrs,
				Timestamp:  time.Now(),
			}

			result := detector.Detect(item, keys, enums)

			if len(result.Entities) != len(tt.expectedTypes) {
				t.Errorf("expected %d entities, got %d", len(tt.expectedTypes), len(result.Entities))
			}

			for _, expectedType := range tt.expectedTypes {
				found := false
				for _, entity := range result.Entities {
					if entity.Type == expectedType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected entity type %s not found", expectedType)
				}
			}

			if len(result.Relations) != tt.expectedRels {
				t.Errorf("expected %d relations, got %d", tt.expectedRels, len(result.Relations))
			}
		})
	}
}

func TestDetectorRegistry(t *testing.T) {
	adp := adapter.NewV1260Adapter()
	registry := NewDetectorRegistry(adp)

	// Test with attributes that should trigger multiple detectors
	item := &model.TelemetryItem{
		SignalType: model.SignalTypeSpan,
		Attrs: model.NormalizedAttrs{
			"service.name":    "order-service",
			"host.name":       "server-1",
			"cloud.region":    "us-east-1",
			"db.system":       "postgresql",
			"db.name":         "orders",
			"server.address":  "db.example.com",
			"k8s.pod.name":    "order-service-xyz",
			"k8s.cluster.name": "prod",
		},
		Timestamp: time.Now(),
	}

	result := registry.Classify(item)

	// Should detect: cloud.region, host, service, db.instance, db.database, k8s.cluster, k8s.pod
	if len(result.Entities) < 5 {
		t.Errorf("expected at least 5 entities, got %d", len(result.Entities))
	}

	// Should have multiple relations
	if len(result.Relations) < 3 {
		t.Errorf("expected at least 3 relations, got %d", len(result.Relations))
	}

	// Verify evidence is populated
	for _, entity := range result.Entities {
		if len(entity.Evidence) == 0 {
			t.Errorf("entity %s (%s) has no evidence", entity.ID, entity.Type)
		}
	}
}
