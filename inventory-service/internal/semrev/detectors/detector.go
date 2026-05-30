// Package detectors implements semantic-conventions based entity detectors.
package detectors

import (
	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// Detector detects entities and relations from telemetry attributes.
type Detector interface {
	// Name returns the detector name for logging.
	Name() string

	// Detect inspects the given attributes and returns classification results.
	Detect(item *model.TelemetryItem, keys adapter.SemconvKeys, enums adapter.SemconvEnums) *model.ClassificationResult
}

// DetectorRegistry holds all registered detectors.
type DetectorRegistry struct {
	detectors []Detector
	adapter   adapter.SemconvAdapter
}

// NewDetectorRegistry creates a new registry with all default detectors.
func NewDetectorRegistry(adapter adapter.SemconvAdapter) *DetectorRegistry {
	return &DetectorRegistry{
		detectors: []Detector{
			NewInfraDetector(),
			NewK8sDetector(),
			NewServiceDetector(),
			NewDatabaseDetector(),
			NewMessagingDetector(),
			NewMobileDetector(),
		},
		adapter: adapter,
	}
}

// Classify runs all detectors on the given telemetry item.
func (r *DetectorRegistry) Classify(item *model.TelemetryItem) *model.ClassificationResult {
	result := &model.ClassificationResult{
		Entities:  make([]*model.Entity, 0),
		Relations: make([]*model.Relation, 0),
	}

	keys := r.adapter.Keys()
	enums := r.adapter.Enums()

	for _, detector := range r.detectors {
		res := detector.Detect(item, keys, enums)
		if res != nil {
			result.Entities = append(result.Entities, res.Entities...)
			result.Relations = append(result.Relations, res.Relations...)
		}
	}

	return result
}

// getAttr safely gets an attribute value.
func getAttr(attrs model.NormalizedAttrs, key string) string {
	if val, ok := attrs[key]; ok {
		return val
	}
	return ""
}

// hasAttr checks if an attribute exists and is non-empty.
func hasAttr(attrs model.NormalizedAttrs, key string) bool {
	val, ok := attrs[key]
	return ok && val != ""
}

// getAttrAny returns the first non-empty attribute from the given keys.
func getAttrAny(attrs model.NormalizedAttrs, keys ...string) string {
	for _, key := range keys {
		if val, ok := attrs[key]; ok && val != "" {
			return val
		}
	}
	return ""
}

// containsString checks if a slice contains a string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// createEvidence creates an evidence record with source provenance.
func createEvidence(item *model.TelemetryItem, semconvVersion, attrKey, attrValue string) model.Evidence {
	return model.Evidence{
		SignalType:     item.SignalType,
		AttributeKey:   attrKey,
		AttributeValue: attrValue,
		SemconvVersion: semconvVersion,
		Timestamp:      item.Timestamp,
		Source:         item.Source, // Copy source info for provenance tracking
	}
}
