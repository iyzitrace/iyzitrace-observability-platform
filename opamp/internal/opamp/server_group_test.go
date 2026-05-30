package opamp

import (
	"context"
	"testing"

	"github.com/getlawrence/lawrence-oss/internal/services"
	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestProcessAgentGrouping_FetchesGroupOTLPSettings(t *testing.T) {
	logger := zap.NewNop()
	agents := NewAgents(logger)
	mockService := new(MockAgentService)

	// Manually construct server to inject mock
	server := &Server{
		logger:       logger,
		agents:       agents,
		agentService: mockService,
	}

	agentID := uuid.New()
	groupID := "group-1"

	// Mock GetGroup
	otlpHTTP := "http://gateway:4318"
	group := &services.Group{
		ID:               groupID,
		OTLPHTTPEndpoint: &otlpHTTP,
	}
	// Expect GetGroup to be called
	mockService.On("GetGroup", mock.Anything, groupID).Return(group, nil)

	// Create Agent
	agent := &Agent{
		InstanceId:    agentID,
		InstanceIdStr: agentID.String(),
		// Initial state: no group
	}

	// Message with group.id
	msg := &protobufs.AgentToServer{
		AgentDescription: &protobufs.AgentDescription{
			IdentifyingAttributes: []*protobufs.KeyValue{
				{
					Key: "group.id",
					Value: &protobufs.AnyValue{
						Value: &protobufs.AnyValue_StringValue{StringValue: groupID},
					},
				},
			},
		},
	}

	// Act
	server.processAgentGrouping(context.Background(), agent, msg)

	// Assert
	assert.Equal(t, &groupID, agent.GroupID)
	assert.Equal(t, &otlpHTTP, agent.GroupOTLPHTTPEndpoint)
	mockService.AssertExpectations(t)
}

func TestCalcConnectionSettings_UsesGroupOTLP(t *testing.T) {
	logger := zap.NewNop()
	// Global config
	server := &Server{
		logger:           logger,
		otlpHTTPEndpoint: "0.0.0.0:4318",
	}

	// Agent with Group Override
	otlpHTTP := "gateway:4318"
	agent := &Agent{
		InstanceIdStr:         "agent-1",
		GroupOTLPHTTPEndpoint: &otlpHTTP,
		Status: &protobufs.AgentToServer{
			Capabilities: uint64(protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnMetrics),
		},
	}

	response := &protobufs.ServerToAgent{}

	// Act
	server.calcConnectionSettings(agent, response)

	// Assert
	// Gateway endpoint should be used
	expectedURL := "http://gateway:4318/v1/metrics"
	assert.NotNil(t, response.ConnectionSettings)
	assert.NotNil(t, response.ConnectionSettings.OwnMetrics)
	assert.Equal(t, expectedURL, response.ConnectionSettings.OwnMetrics.DestinationEndpoint)
}

func TestCalcConnectionSettings_UsesGlobalFallback(t *testing.T) {
	logger := zap.NewNop()
	// Global config
	server := &Server{
		logger:           logger,
		otlpHTTPEndpoint: "0.0.0.0:4318",
	}

	// Agent WITHOUT Group Override
	agent := &Agent{
		InstanceIdStr:         "agent-1",
		GroupOTLPHTTPEndpoint: nil,
		Status: &protobufs.AgentToServer{
			Capabilities: uint64(protobufs.AgentCapabilities_AgentCapabilities_ReportsOwnMetrics),
		},
	}

	response := &protobufs.ServerToAgent{}

	// Act
	server.calcConnectionSettings(agent, response)

	// Assert
	// Global endpoint should be used (mapped to localhost)
	expectedURL := "http://localhost:4318/v1/metrics"
	assert.NotNil(t, response.ConnectionSettings)
	assert.NotNil(t, response.ConnectionSettings.OwnMetrics)
	assert.Equal(t, expectedURL, response.ConnectionSettings.OwnMetrics.DestinationEndpoint)
}
