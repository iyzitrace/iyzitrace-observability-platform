package adapter

// v1260Adapter implements SemconvAdapter for semconv v1.26.0.
type v1260Adapter struct{}

// NewV1260Adapter creates a new adapter for semconv v1.26.0.
func NewV1260Adapter() SemconvAdapter {
	return &v1260Adapter{}
}

// Version returns the semantic conventions version.
func (a *v1260Adapter) Version() string {
	return "1.26.0"
}

// Keys returns the attribute keys for v1.26.0.
// These are based on go.opentelemetry.io/otel/semconv/v1.26.0.
func (a *v1260Adapter) Keys() SemconvKeys {
	return SemconvKeys{
		// Cloud
		CloudProvider:         "cloud.provider",
		CloudRegion:           "cloud.region",
		CloudAvailabilityZone: "cloud.availability_zone",
		CloudAccountID:        "cloud.account.id",
		CloudPlatform:         "cloud.platform",

		// Host
		HostID:   "host.id",
		HostName: "host.name",
		HostType: "host.type",
		HostArch: "host.arch",

		// OS
		OSType:        "os.type",
		OSDescription: "os.description",
		OSName:        "os.name",
		OSVersion:     "os.version",

		// Container
		ContainerID:        "container.id",
		ContainerName:      "container.name",
		ContainerRuntime:   "container.runtime",
		ContainerImageID:   "container.image.id",
		ContainerImageName: "container.image.name",

		// Process
		ProcessPID:            "process.pid",
		ProcessExecutableName: "process.executable.name",
		ProcessExecutablePath: "process.executable.path",
		ProcessCommand:        "process.command",
		ProcessCommandLine:    "process.command_line",
		ProcessOwner:          "process.owner",
		ProcessRuntimeName:    "process.runtime.name",
		ProcessRuntimeVersion: "process.runtime.version",

		// Kubernetes
		K8sClusterName:    "k8s.cluster.name",
		K8sClusterUID:     "k8s.cluster.uid",
		K8sNamespaceName:  "k8s.namespace.name",
		K8sNodeName:       "k8s.node.name",
		K8sNodeUID:        "k8s.node.uid",
		K8sPodName:        "k8s.pod.name",
		K8sPodUID:         "k8s.pod.uid",
		K8sContainerName:  "k8s.container.name",
		K8sDeploymentName: "k8s.deployment.name",
		K8sReplicaSetName: "k8s.replicaset.name",
		K8sStatefulSetName: "k8s.statefulset.name",
		K8sDaemonSetName:  "k8s.daemonset.name",
		K8sJobName:        "k8s.job.name",
		K8sCronJobName:    "k8s.cronjob.name",

		// Service
		ServiceName:       "service.name",
		ServiceNamespace:  "service.namespace",
		ServiceInstanceID: "service.instance.id",
		ServiceVersion:    "service.version",

		// Deployment
		DeploymentEnvironment: "deployment.environment.name",

		// Database
		DBSystem:        "db.system",
		DBName:          "db.name",
		DBNamespace:     "db.namespace",
		DBOperation:     "db.operation",
		DBOperationName: "db.operation.name",
		DBStatement:     "db.statement",
		DBUser:          "db.user",

		// Server/Network
		ServerAddress: "server.address",
		ServerPort:    "server.port",
		NetPeerName:   "net.peer.name",
		NetPeerPort:   "net.peer.port",
		PeerService:   "peer.service",

		// Messaging
		MessagingSystem:          "messaging.system",
		MessagingDestinationName: "messaging.destination.name",
		MessagingDestinationKind: "messaging.destination.kind",
		MessagingOperation:       "messaging.operation",
		MessagingConsumerGroup:   "messaging.consumer.group.name",
		MessagingClientID:        "messaging.client.id",

		// HTTP
		HTTPMethod:        "http.method",
		HTTPRequestMethod: "http.request.method",
		HTTPStatusCode:    "http.status_code",
		HTTPRoute:         "http.route",
		HTTPTarget:        "http.target",
		HTTPURL:           "http.url",
		URLScheme:         "url.scheme",
		URLPath:           "url.path",
		URLFull:           "url.full",

		// RPC
		RPCSystem:  "rpc.system",
		RPCService: "rpc.service",
		RPCMethod:  "rpc.method",

		// Device (mobile)
		DeviceID:           "device.id",
		DeviceModelName:    "device.model.name",
		DeviceManufacturer: "device.manufacturer",
	}
}

// Enums returns the known enumeration values for v1.26.0.
func (a *v1260Adapter) Enums() SemconvEnums {
	return SemconvEnums{
		// Known database systems
		DBSystems: []string{
			"postgresql", "mysql", "mariadb", "mssql", "oracle",
			"db2", "sqlite", "mongodb", "cassandra", "couchdb",
			"cosmosdb", "dynamodb", "elasticsearch", "opensearch",
			"redis", "memcached", "cockroachdb", "clickhouse",
			"spanner", "firestore", "hbase", "neo4j", "influxdb",
			"timescaledb", "questdb", "duckdb", "trino", "presto",
			"hive", "spark", "snowflake", "bigquery", "redshift",
			"other_sql", "unknown",
		},

		// Cache systems (subset of DB systems treated as cache)
		CacheSystems: []string{
			"redis", "memcached", "hazelcast", "aerospike",
		},

		// Messaging systems
		MessagingSystems: []string{
			"kafka", "rabbitmq", "activemq", "aws_sqs", "aws_sns",
			"azure_servicebus", "azure_eventhubs", "gcp_pubsub",
			"pulsar", "rocketmq", "nats", "nsq", "zeromq",
			"solace", "ibmmq", "jms", "other",
		},

		// Cloud providers
		CloudProviders: []string{
			"aws", "azure", "gcp", "alibaba_cloud", "tencent_cloud",
			"ibm_cloud", "oracle_cloud", "digitalocean", "heroku",
		},

		// Operating systems
		OSTypes: []string{
			"linux", "windows", "darwin", "freebsd", "netbsd",
			"openbsd", "dragonflybsd", "hpux", "aix", "solaris",
			"z_os", "ios", "android",
		},

		// Messaging destination kinds
		MessagingDestinationKinds: []string{
			"queue", "topic",
		},
	}
}
