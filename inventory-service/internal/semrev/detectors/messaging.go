package detectors

import (
	"strings"
	"time"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// MessagingDetector detects messaging system entities and relationships.
type MessagingDetector struct{}

// NewMessagingDetector creates a new messaging detector.
func NewMessagingDetector() *MessagingDetector {
	return &MessagingDetector{}
}

// Name returns the detector name.
func (d *MessagingDetector) Name() string {
	return "messaging"
}

// Detect inspects attributes and emits messaging entities and relations.
func (d *MessagingDetector) Detect(item *model.TelemetryItem, keys adapter.SemconvKeys, enums adapter.SemconvEnums) *model.ClassificationResult {
	result := &model.ClassificationResult{
		Entities:  make([]*model.Entity, 0),
		Relations: make([]*model.Relation, 0),
	}

	attrs := item.Attrs
	now := time.Now()

	// Skip if no messaging.system attribute
	if !hasAttr(attrs, keys.MessagingSystem) {
		return result
	}

	msgSystem := strings.ToLower(getAttr(attrs, keys.MessagingSystem))
	destName := getAttr(attrs, keys.MessagingDestinationName)
	destKind := getAttr(attrs, keys.MessagingDestinationKind)
	operation := getAttr(attrs, keys.MessagingOperation)
	serverAddr := getAttrAny(attrs, keys.ServerAddress, keys.NetPeerName)
	serverPort := getAttrAny(attrs, keys.ServerPort, keys.NetPeerPort)

	// Create messaging system entity
	displayName := msgSystem
	if serverAddr != "" {
		displayName = msgSystem + "://" + serverAddr
		if serverPort != "" {
			displayName += ":" + serverPort
		}
	}

	systemBuilder := model.NewEntityBuilder(model.EntityTypeMessagingSystem).
		WithName(displayName).
		WithAttr("messaging.system", msgSystem).
		WithEvidence(createEvidence(item, "1.26.0", keys.MessagingSystem, msgSystem))

	if serverAddr != "" {
		systemBuilder.WithAttr("server.address", serverAddr)
	}
	if serverPort != "" {
		systemBuilder.WithAttr("server.port", serverPort)
	}

	systemEntity := systemBuilder.Build()
	result.Entities = append(result.Entities, systemEntity)

	// Create destination entity if destination name is present
	var destEntity *model.Entity
	if destName != "" {
		kindLabel := destKind
		if kindLabel == "" {
			kindLabel = "destination"
		}

		destDisplayName := destName
		if destKind != "" {
			destDisplayName = destKind + "/" + destName
		}

		destBuilder := model.NewEntityBuilder(model.EntityTypeMessagingDestination).
			WithName(destDisplayName).
			WithAttr("messaging.destination.name", destName).
			WithAttr("messaging.system.id", systemEntity.ID).
			WithEvidence(createEvidence(item, "1.26.0", keys.MessagingDestinationName, destName))

		if destKind != "" {
			destBuilder.WithAttr("messaging.destination.kind", destKind)
		}

		destEntity = destBuilder.Build()
		result.Entities = append(result.Entities, destEntity)

		// Relation: destination part_of system
		result.Relations = append(result.Relations, &model.Relation{
			FromID:    destEntity.ID,
			ToID:      systemEntity.ID,
			Type:      model.RelationTypePartOf,
			FirstSeen: now,
			LastSeen:  now,
			Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.MessagingDestinationName, destName)},
		})
	}

	// Create service relation if service is present
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

		// Determine relation type based on operation
		relationType := model.RelationTypeUses
		if destEntity != nil {
			switch strings.ToLower(operation) {
			case "publish", "send", "produce":
				relationType = model.RelationTypePublishesTo
			case "receive", "process", "consume":
				relationType = model.RelationTypeConsumesFrom
			default:
				relationType = model.RelationTypeUses
			}

			// Service -> destination relation
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    serviceEntityID,
				ToID:      destEntity.ID,
				Type:      relationType,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.MessagingOperation, operation)},
				Attrs: map[string]string{
					"messaging.operation": operation,
				},
			})
		} else {
			// Service uses messaging system (no specific destination)
			result.Relations = append(result.Relations, &model.Relation{
				FromID:    serviceEntityID,
				ToID:      systemEntity.ID,
				Type:      model.RelationTypeUses,
				FirstSeen: now,
				LastSeen:  now,
				Evidence:  []model.Evidence{createEvidence(item, "1.26.0", keys.MessagingSystem, msgSystem)},
				Attrs: map[string]string{
					"messaging.system": msgSystem,
				},
			})
		}
	}

	return result
}
