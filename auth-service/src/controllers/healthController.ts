import { Request, Response } from 'express';
import axios from 'axios';
import http from 'http';
import https from 'https';

interface ComponentStatus {
    name: string;
    status: 'online' | 'degraded' | 'offline';
    latency: string;
    message?: string;
    details?: Record<string, any>;
}

// Helper to format duration
const formatUptime = (startInput: number | string) => {
    let startTimestamp: number;

    if (typeof startInput === 'string') {
        const parsed = Date.parse(startInput);
        if (!isNaN(parsed)) {
            startTimestamp = parsed;
        } else {
            // Try parsing as float string
            startTimestamp = parseFloat(startInput) * 1000;
        }
    } else {
        startTimestamp = startInput * 1000;
    }

    if (isNaN(startTimestamp)) return 'Unknown';

    const uptimeMs = Date.now() - startTimestamp;

    const days = Math.floor(uptimeMs / (1000 * 60 * 60 * 24));
    const hours = Math.floor((uptimeMs / (1000 * 60 * 60)) % 24);
    const minutes = Math.floor((uptimeMs / (1000 * 60)) % 60);
    const seconds = Math.floor((uptimeMs / 1000) % 60);

    return `${days}d ${hours}h ${minutes}m ${seconds}s`;
};

// Flatten nested objects to string values for UI
const flattenObject = (obj: any, prefix = ''): Record<string, string> => {
    let result: Record<string, string> = {};
    for (const key in obj) {
        if (Object.prototype.hasOwnProperty.call(obj, key)) {
            const val = obj[key];
            // Format key to be nicer if possible, but mainly flattening
            const newKey = prefix ? `${prefix} ${key}` : key;
            const safeKey = prefix ? `${prefix}_${key}` : key;

            if (typeof val === 'object' && val !== null) {
                // Recursively flatten
                const flat = flattenObject(val, safeKey);
                result = { ...result, ...flat };
            } else {
                result[safeKey] = String(val);
            }
        }
    }
    return result;
};

const getUptimeFromMetrics = async (baseUrl: string): Promise<{ uptime: string, upsince: string } | null> => {
    try {
        // Most Go apps expose /metrics
        const res = await axios.get(`${baseUrl}/metrics`, { timeout: 1500 });
        const lines = String(res.data).split('\n');
        for (const line of lines) {
            if (line.startsWith('process_start_time_seconds')) {
                const parts = line.split(' ');
                if (parts.length >= 2) {
                    const ts = parseFloat(parts[1]);
                    if (!isNaN(ts)) {
                        return {
                            upsince: new Date(ts * 1000).toISOString(), // ISO 8601 format
                            uptime: formatUptime(ts)
                        };
                    }
                }
            }
        }
    } catch (e) { }
    return null;
};

// Custom agents to prevent socket exhaustion/blocking
const httpAgent = new http.Agent({ keepAlive: false, maxSockets: Infinity });
const httpsAgent = new https.Agent({ keepAlive: false, maxSockets: Infinity });

const checkService = async (url: string, name: string, metricsUrl?: string): Promise<ComponentStatus> => {
    const start = Date.now();
    try {
        // Disable keep-alive (via Agent) to prevent head-of-line blocking if many services are down
        const res = await axios.get(url, {
            timeout: 3000,
            httpAgent,
            httpsAgent,
            validateStatus: () => true // Accept all status codes as "reachable" (we'll check data)
        });

        if (res.status >= 300) {
            throw new Error(`HTTP ${res.status}`);
        }

        const latency = `${Date.now() - start}ms`;

        let rawDetails: any = {};

        if (name === 'Prometheus' && res.data?.data) {
            rawDetails = res.data.data;
            // Handle Prometheus uptime
            if (rawDetails.startTime) {
                rawDetails.uptime = formatUptime(rawDetails.startTime);
                // Standardize to Upsince
                rawDetails.Upsince = rawDetails.startTime;
                delete rawDetails.startTime;
            }
        } else if (name === 'SeaweedFS') {
            const data = res.data;
            rawDetails = {};
            if (data.Version) rawDetails['Version'] = data.Version;
            if (data.Leader) rawDetails['Leader'] = data.Leader;

            // Extract Topology stats strictly from the first call if available
            if (data.Topology) {
                const topo = data.Topology;
                if (topo.Free !== undefined) rawDetails['Free_Volumes'] = String(topo.Free);
                if (topo.Max !== undefined) rawDetails['Max_Volumes'] = String(topo.Max);
                if (topo.DataCenters && Array.isArray(topo.DataCenters)) rawDetails['DataCenters'] = String(topo.DataCenters.length);
                if (topo.Racks && Array.isArray(topo.Racks)) rawDetails['Racks'] = String(topo.Racks.length);
                if (topo.DataNodes && Array.isArray(topo.DataNodes)) rawDetails['DataNodes'] = String(topo.DataNodes.length);
            }
        } else if (typeof res.data === 'object' && res.data !== null) {
            rawDetails = res.data;
        } else {
            rawDetails = { response: String(res.data) };
        }

        // Standardize OTel Collector "upSince" to "Upsince" and fix uptime format
        if (rawDetails.upSince) {
            rawDetails.Upsince = rawDetails.upSince;
            delete rawDetails.upSince;
            // Overwrite raw uptime (e.g. "15m30s") with our standard format
            rawDetails.uptime = formatUptime(rawDetails.Upsince);
        }

        // Add Check URL field
        rawDetails['Check_URL'] = url;

        // Attempt to fetch uptime via metrics if not already present
        if (metricsUrl && !rawDetails.uptime) {
            const uptimeInfo = await getUptimeFromMetrics(metricsUrl);
            if (uptimeInfo) {
                rawDetails.uptime = uptimeInfo.uptime;
                rawDetails.Upsince = uptimeInfo.upsince;
            }
        }

        // Flatten for UI
        const details = flattenObject(rawDetails);

        return { name, status: 'online', latency, details };
    } catch (err: any) {
        const latency = `${Date.now() - start}ms`;
        return { name, status: 'offline', latency, message: err.message };
    }
};

export const getSystemStatus = async (req: Request, res: Response) => {
    // Targets with optional separate metrics URL for uptime
    const targets: Record<string, [string, string?]> = {
        'Loki': [
            process.env.LOKI_HEALTH_URL || 'http://loki:3100/loki/api/v1/status/buildinfo',
            process.env.LOKI_METRICS_URL || 'http://loki:3100'
        ],
        'Tempo': [
            process.env.TEMPO_HEALTH_URL || 'http://tempo:3200/api/status/buildinfo',
            process.env.TEMPO_METRICS_URL || 'http://tempo:3200'
        ],
        'Prometheus': [
            process.env.PROMETHEUS_HEALTH_URL || 'http://prometheus:9090/api/v1/status/buildinfo',
            process.env.PROMETHEUS_METRICS_URL || 'http://prometheus:9090'
        ],
        'SeaweedFS': [
            process.env.SEAWEEDFS_HEALTH_URL || 'http://seaweedfs:9333/cluster/status',
            process.env.SEAWEEDFS_METRICS_URL || undefined
        ],
        'Log Collector': [
            process.env.LOG_COLLECTOR_HEALTH_URL || 'http://log-otel-enrichment:13133/',
            process.env.LOG_COLLECTOR_METRICS_URL || 'http://log-otel-enrichment:8888'
        ],
        'Metric Collector': [
            process.env.METRIC_COLLECTOR_HEALTH_URL || 'http://metric-otel-enrichment:13133/',
            process.env.METRIC_COLLECTOR_METRICS_URL || 'http://metric-otel-enrichment:8888'
        ],
        'Trace Collector': [
            process.env.TRACE_COLLECTOR_HEALTH_URL || 'http://trace-otel-enrichment:13133/',
            process.env.TRACE_COLLECTOR_METRICS_URL || 'http://trace-otel-enrichment:8888'
        ],
        'OpAMP Server': [
            process.env.OPAMP_SERVER_HEALTH_URL || 'http://lawrence:8080/health',
            process.env.OPAMP_SERVER_METRICS_URL || 'http://lawrence:8080'
        ],
    };

    let overallStatus: 'healthy' | 'degraded' | 'unhealthy' = 'healthy';

    // Run checks in parallel
    const components = await Promise.all(Object.entries(targets).map(async ([name, [url, metricsUrl]]) => {
        let status = await checkService(url, name, metricsUrl);

        // SeaweedFS Extra: Check /dir/status to GUARANTEE we get Volume/Topology info
        // /cluster/status might not have full topology details depending on version/config.
        if (name === 'SeaweedFS' && status.status === 'online') {
            try {
                const dirStatusURL = process.env.SEAWEEDFS_DIR_STATUS_URL || 'http://seaweedfs:9333/dir/status';
                const dirRes = await axios.get(dirStatusURL, { timeout: 1000 });
                const data = dirRes.data;

                // Merge Version if checkService didn't find it
                if (!status.details?.Version && data.Version) {
                    status.details = { ...status.details, Version: data.Version };
                }

                // Explicitly extract Topology stats from /dir/status if present
                if (data.Topology) {
                    const topo = data.Topology;
                    if (topo.Free !== undefined) status.details!.Free_Volumes = String(topo.Free);
                    if (topo.Max !== undefined) status.details!.Max_Volumes = String(topo.Max);
                    if (topo.DataCenters && Array.isArray(topo.DataCenters)) status.details!.DataCenters = String(topo.DataCenters.length);
                    if (topo.Racks && Array.isArray(topo.Racks)) status.details!.Racks = String(topo.Racks.length);
                    if (topo.DataNodes && Array.isArray(topo.DataNodes)) status.details!.DataNodes = String(topo.DataNodes.length);
                }
            } catch (ignored) { }
        }

        if (status.status !== 'online') overallStatus = 'unhealthy';
        return status;
    }));

    const response = {
        status: overallStatus,
        components,
        updated_at: new Date().toISOString()
    };

    res.json(response);
};
