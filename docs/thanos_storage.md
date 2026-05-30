# Thanos Storage Configuration & Sizing

## 1. Local Disk Sizing (Prometheus)

When using Thanos Sidecar, Prometheus needs enough local disk to store data until it is uploaded to object storage (usually every 2 hours).

### Formula
`Required Disk = (Retention Time buffer) * (Ingestion Rate)`

For a target of **500,000 metrics/second** (samples/sec):
- **Sample Size**: ~1.5 bytes per sample (Gorilla compression average)
- **Ingestion Rate**: 500k * 1.5 bytes = 750 KB/sec
- **Hourly Rate**: 750 KB * 3600 = ~2.7 GB/hour

**Buffer**:
We configure Prometheus with `storage.tsdb.retention.time=2h`.
To be safe (compaction overhead + WAL), we recommend **3x the retention period**:

`Disk Space = 3 * 2 hours * 2.7 GB/hour = ~16.2 GB`

**Recommendation**: Allocate at least **20 GB** for the local Prometheus volume.

---

## 2. Retention Configuration (Thanos Compactor)

Thanos Compactor handles the retention of historical data in object storage. This is configured via command-line flags in `docker-compose.yml`.

### Current Configuration (300GB/month target)

```yaml
thanos-compactor:
  command:
    - compact
    - --retention.resolution-raw=14d   # Keep high-precision raw data for 14 days
    - --retention.resolution-5m=90d    # Keep 5-minute downsampled data for 90 days
    - --retention.resolution-1h=1y     # Keep 1-hour downsampled data for 1 year
```

### How to Modify
1. Open `docker-compose.yml`.
2. Locate the `thanos-compactor` service.
3. Change the values for:
   - `--retention.resolution-raw`: Raw samples (most expensive storage).
   - `--retention.resolution-5m`: Mid-term trends.
   - `--retention.resolution-1h`: Long-term trends (very cheap).
4. Restart the service: `docker-compose restart thanos-compactor`.

### Storage Cost Estimation (Monthly)
- **Raw**: 14 days * 24h * 2.7 GB = ~900 GB (rolling window)
- **5m Downsample**: ~10% size of raw = ~270 MB/hour
- **1h Downsample**: ~0.8% size of raw = ~20 MB/hour

*Note: Downsampling massively reduces long-term storage costs.*
