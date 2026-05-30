package detectors

import (
	"strings"
	"time"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// DatabaseDetector detects database entities and relationships.
type DatabaseDetector struct{}

// NewDatabaseDetector creates a new database detector.
func NewDatabaseDetector() *DatabaseDetector {
	return &DatabaseDetector{}
}

// Name returns the detector name.
func (d *DatabaseDetector) Name() string {
	return "database"
}

// Detect inspects attributes and emits database entities and relations.
func (d *DatabaseDetector) Detect(item *model.TelemetryItem, keys adapter.SemconvKeys, enums adapter.SemconvEnums) *model.ClassificationResult {
	result := &model.ClassificationResult{
		Entities:  make([]*model.Entity, 0),
		Relations: make([]*model.Relation, 0),
	}

	attrs := item.Attrs
	now := time.Now()

	// Skip if no db.system attribute
	if !hasAttr(attrs, keys.DBSystem) {
		return result
	}

	dbSystem := strings.ToLower(getAttr(attrs, keys.DBSystem))
	dbName := getAttr(attrs, keys.DBName)
	serverAddr := getAttrAny(attrs, keys.ServerAddress, keys.NetPeerName, keys.PeerService)
	serverPort := getAttrAny(attrs, keys.ServerPort, keys.NetPeerPort)

	// Determine if this is a cache or database
	isCache := containsString(enums.CacheSystems, dbSystem)

	var instanceEntity *model.Entity

	if isCache {
		// Create cache instance entity
		displayName := dbSystem
		if serverAddr != "" {
			displayName = dbSystem + "://" + serverAddr
			if serverPort != "" {
				displayName += ":" + serverPort
			}
		}

		builder := model.NewEntityBuilder(model.EntityTypeCacheInstance).
			WithName(displayName).
			WithAttr("db.system", dbSystem).
			WithEvidence(createEvidence(item, "1.26.0", keys.DBSystem, dbSystem))

		if serverAddr != "" {
			builder.WithAttr("server.address", serverAddr)
		}
		if serverPort != "" {
			builder.WithAttr("server.port", serverPort)
		}

		instanceEntity = builder.Build()
	} else {
		// Create database instance entity
		displayName := dbSystem
		if serverAddr != "" {
			displayName = dbSystem + "://" + serverAddr
			if serverPort != "" {
				displayName += ":" + serverPort
			}
		}

		builder := model.NewEntityBuilder(model.EntityTypeDBInstance).
			WithName(displayName).
			WithAttr("db.system", dbSystem).
			WithEvidence(createEvidence(item, "1.26.0", keys.DBSystem, dbSystem))

		if serverAddr != "" {
			builder.WithAttr("server.address", serverAddr)
		}
		if serverPort != "" {
			builder.WithAttr("server.port", serverPort)
		}

		instanceEntity = builder.Build()
	}

	result.Entities = append(result.Entities, instanceEntity)

	// Create database entity if db.name is present
	var dbEntity *model.Entity
	if dbName != "" {
		builder := model.NewEntityBuilder(model.EntityTypeDBDatabase).
			WithName(dbName).
			WithAttr("db.name", dbName).
			WithAttr("db.instance.id", instanceEntity.ID).
			WithEvidence(createEvidence(item, "1.26.0", keys.DBName, dbName))

		dbEntity = builder.Build()
		result.Entities = append(result.Entities, dbEntity)

		// Relation: database belongs_to instance
		result.Relations = append(result.Relations, &model.Relation{
			FromID:    dbEntity.ID,
			ToID:      instanceEntity.ID,
			Type:      model.RelationTypeBelongsTo,
			FirstSeen: now,
			LastSeen:  now,
			Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.DBName, dbName)},
		})
	}

	// Create service uses db/cache relation if service is present
	if hasAttr(attrs, keys.ServiceName) {
		serviceName := getAttr(attrs, keys.ServiceName)
		serviceNamespace := getAttr(attrs, keys.ServiceNamespace)

		serviceAttrs := map[string]string{
			"service.name": serviceName,
		}
		if serviceNamespace != "" {
			serviceAttrs["service.namespace"] = serviceNamespace
		}
		serviceEntityID := model.GenerateEntityID(model.EntityTypeService, serviceAttrs)

		// Service uses db/cache instance
		result.Relations = append(result.Relations, &model.Relation{
			FromID:    serviceEntityID,
			ToID:      instanceEntity.ID,
			Type:      model.RelationTypeUses,
			FirstSeen: now,
			LastSeen:  now,
			Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.DBSystem, dbSystem)},
			Attrs: map[string]string{
				"db.system": dbSystem,
			},
		})
	}

	return result
}
