// Package adapter provides version-aware semantic conventions adapters.
package adapter

// SemconvKeys contains attribute key constants relevant to detectors.
type SemconvKeys struct {
	// Cloud
	CloudProvider         string
	CloudRegion           string
	CloudAvailabilityZone string
	CloudAccountID        string
	CloudPlatform         string

	// Host
	HostID   string
	HostName string
	HostType string
	HostArch string

	// OS
	OSType        string
	OSDescription string
	OSName        string
	OSVersion     string

	// Container
	ContainerID        string
	ContainerName      string
	ContainerRuntime   string
	ContainerImageID   string
	ContainerImageName string

	// Process
	ProcessPID            string
	ProcessExecutableName string
	ProcessExecutablePath string
	ProcessCommand        string
	ProcessCommandLine    string
	ProcessOwner          string
	ProcessRuntimeName    string
	ProcessRuntimeVersion string

	// Kubernetes
	K8sClusterName    string
	K8sClusterUID     string
	K8sNamespaceName  string
	K8sNodeName       string
	K8sNodeUID        string
	K8sPodName        string
	K8sPodUID         string
	K8sContainerName  string
	K8sDeploymentName string
	K8sReplicaSetName string
	K8sStatefulSetName string
	K8sDaemonSetName  string
	K8sJobName        string
	K8sCronJobName    string

	// Service
	ServiceName       string
	ServiceNamespace  string
	ServiceInstanceID string
	ServiceVersion    string

	// Deployment
	DeploymentEnvironment string

	// Database
	DBSystem         string
	DBName           string
	DBNamespace      string
	DBOperation      string
	DBOperationName  string
	DBStatement      string
	DBUser           string

	// Server/Network
	ServerAddress string
	ServerPort    string
	NetPeerName   string
	NetPeerPort   string
	PeerService   string

	// Messaging
	MessagingSystem          string
	MessagingDestinationName string
	MessagingDestinationKind string
	MessagingOperation       string
	MessagingConsumerGroup   string
	MessagingClientID        string

	// HTTP
	HTTPMethod        string
	HTTPRequestMethod string
	HTTPStatusCode    string
	HTTPRoute         string
	HTTPTarget        string
	HTTPURL           string
	URLScheme         string
	URLPath           string
	URLFull           string

	// RPC
	RPCSystem  string
	RPCService string
	RPCMethod  string

	// Device (mobile)
	DeviceID           string
	DeviceModelName    string
	DeviceManufacturer string
}

// SemconvEnums contains known enumeration values for semantic conventions.
type SemconvEnums struct {
	// Known database systems
	DBSystems []string

	// Cache systems (subset of DB systems treated as cache)
	CacheSystems []string

	// Messaging systems
	MessagingSystems []string

	// Cloud providers
	CloudProviders []string

	// Operating systems
	OSTypes []string

	// Messaging destination kinds
	MessagingDestinationKinds []string
}

// SemconvAdapter provides version-specific semantic conventions.
type SemconvAdapter interface {
	// Version returns the semantic conventions version.
	Version() string

	// Keys returns the attribute keys for this version.
	Keys() SemconvKeys

	// Enums returns the known enumeration values for this version.
	Enums() SemconvEnums
}

// DefaultAdapter returns the default (latest) semconv adapter.
func DefaultAdapter() SemconvAdapter {
	return NewV1260Adapter()
}

// GetAdapter returns the adapter for the specified version.
// If the version is not supported, it returns the default adapter.
func GetAdapter(version string) SemconvAdapter {
	switch version {
	case "1.26.0":
		return NewV1260Adapter()
	default:
		return DefaultAdapter()
	}
}
