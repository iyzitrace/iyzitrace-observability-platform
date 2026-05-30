package model

import (
	"testing"
)

func TestGenerateEntityID_Robustness(t *testing.T) {
	tests := []struct {
		name       string
		entityType EntityType
		attrs1     map[string]string
		attrs2     map[string]string
		shouldMatch bool
	}{
		{
			name:       "Service case insensitivity",
			entityType: EntityTypeService,
			attrs1:     map[string]string{"service.name": "PaymentService", "service.namespace": "Prod"},
			attrs2:     map[string]string{"service.name": "paymentservice", "service.namespace": "prod"},
			shouldMatch: true,
		},
		{
			name:       "Host prefer name over ID",
			entityType: EntityTypeHost,
			attrs1:     map[string]string{"host.name": "web-01", "host.id": "i-123456"},
			attrs2:     map[string]string{"host.name": "web-01"}, // Missing ID
			shouldMatch: true, // Should match because both use host.name
		},
		{
			name:       "Host fallback to ID",
			entityType: EntityTypeHost,
			attrs1:     map[string]string{"host.id": "i-9999"},
			attrs2:     map[string]string{"host.id": "i-9999"},
			shouldMatch: true,
		},
		{
			name:       "Host different names are different",
			entityType: EntityTypeHost,
			attrs1:     map[string]string{"host.name": "web-01"},
			attrs2:     map[string]string{"host.name": "web-02"},
			shouldMatch: false,
		},
		{
			name:       "Container ID dominance",
			entityType: EntityTypeContainer,
			attrs1:     map[string]string{"container.id": "abc12345", "container.name": "redis"},
			attrs2:     map[string]string{"container.id": "abc12345", "container.name": "redis-container"},
			shouldMatch: true, // ID matches, name ignored for ID generation
		},
		{
			name:       "Cloud Region format",
			entityType: EntityTypeCloudRegion,
			attrs1:     map[string]string{"cloud.provider": "AWS", "cloud.region": "us-east-1"},
			attrs2:     map[string]string{"cloud.provider": "aws", "cloud.region": "US-EAST-1"},
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id1 := GenerateEntityID(tt.entityType, tt.attrs1)
			id2 := GenerateEntityID(tt.entityType, tt.attrs2)

			if tt.shouldMatch && id1 != id2 {
				t.Errorf("IDs should match but didn't: %s vs %s", id1, id2)
			}
			if !tt.shouldMatch && id1 == id2 {
				t.Errorf("IDs should NOT match but did: %s", id1)
			}
		})
	}
}
