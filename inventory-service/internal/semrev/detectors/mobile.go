package detectors

import (
	"strings"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// MobileDetector detects mobile application entities.
type MobileDetector struct{}

// NewMobileDetector creates a new mobile detector.
func NewMobileDetector() *MobileDetector {
	return &MobileDetector{}
}

// Name returns the detector name.
func (d *MobileDetector) Name() string {
	return "mobile"
}

// Detect inspects attributes and emits mobile app entities and relations.
func (d *MobileDetector) Detect(item *model.TelemetryItem, keys adapter.SemconvKeys, enums adapter.SemconvEnums) *model.ClassificationResult {
	result := &model.ClassificationResult{
		Entities:  make([]*model.Entity, 0),
		Relations: make([]*model.Relation, 0),
	}

	attrs := item.Attrs

	// Check for mobile OS type
	osType := strings.ToLower(getAttr(attrs, keys.OSType))
	isMobile := osType == "ios" || osType == "android"

	// Also check for device attributes as indicators
	hasDevice := hasAttr(attrs, keys.DeviceID) ||
		hasAttr(attrs, keys.DeviceModelName) ||
		hasAttr(attrs, keys.DeviceManufacturer)

	if !isMobile && !hasDevice {
		return result
	}

	// Need a service name to create a mobile app entity
	if !hasAttr(attrs, keys.ServiceName) {
		return result
	}

	serviceName := getAttr(attrs, keys.ServiceName)
	serviceVersion := getAttr(attrs, keys.ServiceVersion)
	deviceID := getAttr(attrs, keys.DeviceID)
	deviceModel := getAttr(attrs, keys.DeviceModelName)
	deviceManufacturer := getAttr(attrs, keys.DeviceManufacturer)

	// Determine entity type based on OS
	var entityType model.EntityType
	switch osType {
	case "ios":
		entityType = model.EntityTypeMobileAppIOS
	case "android":
		entityType = model.EntityTypeMobileAppAndroid
	default:
		entityType = model.EntityTypeMobileApp
	}

	displayName := serviceName
	if osType != "" {
		displayName = serviceName + " (" + osType + ")"
	}

	builder := model.NewEntityBuilder(entityType).
		WithName(displayName).
		WithAttr("service.name", serviceName).
		WithEvidence(createEvidence(item, "1.26.0", keys.OSType, osType))

	if osType != "" {
		builder.WithAttr("os.type", osType)
	}
	if serviceVersion != "" {
		builder.WithAttr("service.version", serviceVersion)
	}
	if deviceID != "" {
		builder.WithAttr("device.id", deviceID)
	}
	if deviceModel != "" {
		builder.WithAttr("device.model.name", deviceModel)
	}
	if deviceManufacturer != "" {
		builder.WithAttr("device.manufacturer", deviceManufacturer)
	}

	mobileEntity := builder.Build()
	result.Entities = append(result.Entities, mobileEntity)

	return result
}
