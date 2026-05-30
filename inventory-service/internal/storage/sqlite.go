package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewSQLiteStore creates a new SQLite store.
func NewSQLiteStore(dbPath string, logger *zap.Logger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStore{
		db:     db,
		logger: logger.Named("sqlite"),
	}

	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

// migrate creates the database schema.
func (s *SQLiteStore) migrate() error {
	// Create schema version table
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	// Check current version
	var version int
	err = s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	s.logger.Info("Current schema version", zap.Int("version", version))

	// Run migrations
	if version < 1 {
		s.logger.Info("Running migration to version 1")
		// Drop old tables if they exist (to fix schema issues)
		_, _ = s.db.Exec(`DROP TABLE IF EXISTS relations`)
		_, _ = s.db.Exec(`DROP TABLE IF EXISTS entities`)

		schema := `
		CREATE TABLE entities (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			attrs TEXT NOT NULL,
			first_seen DATETIME NOT NULL,
			last_seen DATETIME NOT NULL,
			evidence TEXT NOT NULL
		);

		CREATE INDEX idx_entities_type ON entities(type);
		CREATE INDEX idx_entities_name ON entities(name);

		CREATE TABLE relations (
			id TEXT PRIMARY KEY,
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL,
			type TEXT NOT NULL,
			attrs TEXT,
			first_seen DATETIME NOT NULL,
			last_seen DATETIME NOT NULL,
			evidence TEXT NOT NULL,
			UNIQUE(from_id, to_id, type)
		);

		CREATE INDEX idx_relations_from ON relations(from_id);
		CREATE INDEX idx_relations_to ON relations(to_id);
		CREATE INDEX idx_relations_type ON relations(type);

		INSERT INTO schema_version (version) VALUES (1);
		`

		if _, err := s.db.Exec(schema); err != nil {
			return fmt.Errorf("failed to run migration v1: %w", err)
		}
		s.logger.Info("Migration to version 1 complete")
		version = 1
	}

	// Migration v2: Add status and instance tracking columns
	if version < 2 {
		s.logger.Info("Running migration to version 2")

		migrations := []string{
			`ALTER TABLE entities ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`,
			`ALTER TABLE entities ADD COLUMN instance_count INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE entities ADD COLUMN active_count INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE entities ADD COLUMN instances TEXT NOT NULL DEFAULT '[]'`,
			`CREATE INDEX idx_entities_status ON entities(status)`,
			`INSERT INTO schema_version (version) VALUES (2)`,
		}

		for _, m := range migrations {
			if _, err := s.db.Exec(m); err != nil {
				// Ignore errors for columns that already exist
				s.logger.Warn("Migration statement failed (may already exist)", zap.String("sql", m), zap.Error(err))
			}
		}
		s.logger.Info("Migration to version 2 complete")
	}

	return nil
}

// UpsertEntities inserts or updates entities.
func (s *SQLiteStore) UpsertEntities(ctx context.Context, entities []*model.Entity) error {
	if len(entities) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Bulk fetch existing data to avoid N+1 queries
	ids := make([]string, len(entities))
	for i, e := range entities {
		ids[i] = e.ID
	}

	existingSnapshots, err := s.getEntitiesSnapshotBulk(ctx, tx, ids)
	if err != nil {
		s.logger.Error("Failed to bulk load entity snapshots", zap.Error(err))
		return err
	}

	existingInstancesMap, err := s.getEntityInstancesBulk(ctx, tx, ids)
	if err != nil {
		s.logger.Error("Failed to bulk load entity instances", zap.Error(err))
		return err
	}

	// Prepare statement for upsert
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO entities (id, type, name, attrs, first_seen, last_seen, evidence, status, instance_count, active_count, instances)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			attrs = excluded.attrs,
			last_seen = excluded.last_seen,
			evidence = excluded.evidence,
			status = 'active',
			instance_count = excluded.instance_count,
			active_count = excluded.active_count,
			instances = excluded.instances
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, entity := range entities {
		// Load existing snapshot from map
		existing, found := existingSnapshots[entity.ID]
		if !found {
			// New entity
			existing = &entitySnapshot{
				Name:     "",
				Attrs:    map[string]string{},
				Evidence: []model.Evidence{},
			}
		}

		mergedName := mergeName(existing.Name, entity.Name)
		mergedAttrs, changedKeys := mergeAttrsWithChanges(existing.Attrs, entity.Attrs)
		mergedEvidence := mergeEvidence(existing.Evidence, entity.Evidence, changedKeys, 120, 2)

		attrsJSON, err := json.Marshal(mergedAttrs)
		if err != nil {
			s.logger.Warn("Failed to marshal entity attrs", zap.Error(err))
			continue
		}

		evidenceJSON, err := json.Marshal(mergedEvidence)
		if err != nil {
			s.logger.Warn("Failed to marshal entity evidence", zap.Error(err))
			continue
		}

		// Get existing instances from map
		existingInstances := existingInstancesMap[entity.ID]
		if existingInstances == nil {
			existingInstances = []model.InstanceInfo{}
		}

		// Merge new instances with existing ones
		mergedInstances := mergeInstances(existingInstances, entity.Instances)

		// Limit to last 100 instances
		if len(mergedInstances) > 100 {
			mergedInstances = mergedInstances[len(mergedInstances)-100:]
		}

		instancesJSON, err := json.Marshal(mergedInstances)
		if err != nil {
			s.logger.Warn("Failed to marshal entity instances", zap.Error(err))
			instancesJSON = []byte("[]")
		}

		// Count active instances
		activeCount := 0
		for _, inst := range mergedInstances {
			if inst.Status == model.EntityStatusActive {
				activeCount++
			}
		}

		status := entity.Status
		if status == "" {
			status = model.EntityStatusActive
		}

		_, err = stmt.ExecContext(ctx,
			entity.ID,
			string(entity.Type),
			mergedName,
			string(attrsJSON),
			entity.FirstSeen,
			entity.LastSeen,
			string(evidenceJSON),
			string(status),
			len(mergedInstances), // Total unique instances tracked
			activeCount,
			string(instancesJSON),
		)
		if err != nil {
			s.logger.Warn("Failed to upsert entity", zap.String("id", entity.ID), zap.Error(err))
		}
	}

	return tx.Commit()
}

type entitySnapshot struct {
	Name     string
	Attrs    map[string]string
	Evidence []model.Evidence
}

func (s *SQLiteStore) getEntitiesSnapshotBulk(ctx context.Context, tx *sql.Tx, ids []string) (map[string]*entitySnapshot, error) {
	if len(ids) == 0 {
		return make(map[string]*entitySnapshot), nil
	}

	// Deduplicate IDs
	uniqueIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		uniqueIDs[id] = struct{}{}
	}

	// Batch in chunks of 500 to avoid query limits
	result := make(map[string]*entitySnapshot, len(uniqueIDs))
	chunk := make([]interface{}, 0, 500)
	placeholders := make([]string, 0, 500)

	idList := make([]string, 0, len(uniqueIDs))
	for id := range uniqueIDs {
		idList = append(idList, id)
	}

	for i := 0; i < len(idList); i += 500 {
		chunk = chunk[:0]
		placeholders = placeholders[:0]
		end := i + 500
		if end > len(idList) {
			end = len(idList)
		}

		for _, id := range idList[i:end] {
			chunk = append(chunk, id)
			placeholders = append(placeholders, "?")
		}

		query := "SELECT id, name, attrs, evidence FROM entities WHERE id IN (" + strings.Join(placeholders, ",") + ")"
		rows, err := tx.QueryContext(ctx, query, chunk...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id, name, attrsJSON, evidenceJSON string
			if err := rows.Scan(&id, &name, &attrsJSON, &evidenceJSON); err != nil {
				continue
			}

			var attrs map[string]string
			if err := json.Unmarshal([]byte(attrsJSON), &attrs); err != nil {
				attrs = map[string]string{}
			}

			var evidence []model.Evidence
			if err := json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
				evidence = []model.Evidence{}
			}

			result[id] = &entitySnapshot{
				Name:     name,
				Attrs:    attrs,
				Evidence: evidence,
			}
		}
	}

	return result, nil
}

// Keep single getEntitySnapshot for backward compatibility or individual lookups if needed
func (s *SQLiteStore) getEntitySnapshot(ctx context.Context, tx *sql.Tx, entityID string) (*entitySnapshot, error) {
	var name, attrsJSON, evidenceJSON string
	err := tx.QueryRowContext(ctx, `SELECT name, attrs, evidence FROM entities WHERE id = ?`, entityID).Scan(&name, &attrsJSON, &evidenceJSON)
	if err != nil {
		return nil, err
	}

	var attrs map[string]string
	if err := json.Unmarshal([]byte(attrsJSON), &attrs); err != nil {
		attrs = map[string]string{}
	}

	var evidence []model.Evidence
	if err := json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
		evidence = []model.Evidence{}
	}

	return &entitySnapshot{
		Name:     name,
		Attrs:    attrs,
		Evidence: evidence,
	}, nil
}

// mergeName prefers a non-empty, more descriptive name.
func mergeName(existing, incoming string) string {
	if strings.TrimSpace(incoming) == "" {
		return existing
	}
	if strings.TrimSpace(existing) == "" {
		return incoming
	}
	// Prefer the longer name (often includes namespace/prefix).
	if len(incoming) > len(existing) {
		return incoming
	}
	return existing
}

// mergeAttrs fills missing attributes and avoids overwriting richer values with sparse ones.
func mergeAttrs(existing, incoming map[string]string) map[string]string {
	merged, _ := mergeAttrsWithChanges(existing, incoming)
	return merged
}

// mergeAttrsWithChanges merges attributes and returns which keys changed (added/overridden).
func mergeAttrsWithChanges(existing, incoming map[string]string) (map[string]string, map[string]struct{}) {
	out := make(map[string]string, len(existing)+len(incoming))
	changed := make(map[string]struct{})
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range incoming {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		cur := strings.TrimSpace(out[k])
		if shouldOverrideAttr(cur, v) {
			out[k] = v
			changed[k] = struct{}{}
		}
	}
	return out, changed
}

func shouldOverrideAttr(existing, incoming string) bool {
	if existing == "" {
		return true
	}
	low := strings.ToLower(existing)
	switch low {
	case "unknown", "n/a", "na", "-", "none", "null":
		return true
	}
	// If incoming looks more specific (longer), allow override.
	if len(incoming) > len(existing) && !strings.EqualFold(existing, incoming) {
		return true
	}
	return false
}

// mergeEvidence keeps evidence only when it adds enrichment (or first proof),
// and de-dupes + caps aggressively to avoid DB growth.
func mergeEvidence(existing, incoming []model.Evidence, changedAttrKeys map[string]struct{}, maxTotal, maxPerAttrKey int) []model.Evidence {
	// Always prune existing first.
	existing = pruneEvidence(existing, maxTotal, maxPerAttrKey)

	seen := make(map[string]struct{}, len(existing)+len(incoming))
	byAttrKeyCount := make(map[string]int)
	hasAttrKey := make(map[string]bool)

	keyOf := func(e model.Evidence) string {
		// De-dupe by semantic evidence fields + signal source identifiers.
		var src string
		if e.Source != nil {
			src = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
				e.Source.SignalType,
				e.Source.TraceID,
				e.Source.SpanID,
				e.Source.MetricName,
				e.Source.LogSeverity,
				e.Source.ServiceName,
				e.Source.ServiceNamespace,
			)
		}
		return fmt.Sprintf("%s|%s|%s|%s|%s", e.SignalType, e.AttributeKey, e.AttributeValue, e.SemconvVersion, src)
	}

	out := make([]model.Evidence, 0, len(existing)+len(incoming))
	for _, e := range existing {
		k := keyOf(e)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, e)
		byAttrKeyCount[e.AttributeKey]++
		hasAttrKey[e.AttributeKey] = true
	}

	// Sort incoming by timestamp so we keep the most recent enrichment proof.
	sort.SliceStable(incoming, func(i, j int) bool {
		return incoming[i].Timestamp.Before(incoming[j].Timestamp)
	})

	for _, e := range incoming {
		// Skip empty evidence keys
		if strings.TrimSpace(e.AttributeKey) == "" || strings.TrimSpace(e.AttributeValue) == "" {
			continue
		}

		k := keyOf(e)
		if _, ok := seen[k]; ok {
			continue
		}

		keep := false
		if len(changedAttrKeys) > 0 {
			// Only keep evidence for attributes that were enriched/changed.
			_, keep = changedAttrKeys[e.AttributeKey]
		} else {
			// If nothing changed, keep at most one "first proof" per attribute key.
			keep = !hasAttrKey[e.AttributeKey]
		}

		if !keep {
			continue
		}

		// Cap per attribute key.
		if maxPerAttrKey > 0 && byAttrKeyCount[e.AttributeKey] >= maxPerAttrKey {
			continue
		}

		out = append(out, e)
		seen[k] = struct{}{}
		byAttrKeyCount[e.AttributeKey]++
		hasAttrKey[e.AttributeKey] = true

		// Cap total.
		if maxTotal > 0 && len(out) >= maxTotal {
			break
		}
	}

	// Final prune to be safe.
	return pruneEvidence(out, maxTotal, maxPerAttrKey)
}

// errorsIsNoRows is a small helper to avoid importing errors everywhere.
func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func pruneEvidence(evidence []model.Evidence, maxTotal, maxPerAttrKey int) []model.Evidence {
	if len(evidence) == 0 {
		return evidence
	}
	// Sort by timestamp asc (oldest first), we will keep the newest within limits.
	sort.SliceStable(evidence, func(i, j int) bool {
		return evidence[i].Timestamp.Before(evidence[j].Timestamp)
	})

	// Walk newest->oldest to keep latest evidence.
	byKey := make(map[string]int)
	out := make([]model.Evidence, 0, len(evidence))
	for i := len(evidence) - 1; i >= 0; i-- {
		e := evidence[i]
		if maxPerAttrKey > 0 && byKey[e.AttributeKey] >= maxPerAttrKey {
			continue
		}
		out = append(out, e)
		byKey[e.AttributeKey]++
		if maxTotal > 0 && len(out) >= maxTotal {
			break
		}
	}
	// Reverse back to chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// getEntityInstancesBulk retrieves instances for multiple entities at once.
func (s *SQLiteStore) getEntityInstancesBulk(ctx context.Context, tx *sql.Tx, ids []string) (map[string][]model.InstanceInfo, error) {
	if len(ids) == 0 {
		return make(map[string][]model.InstanceInfo), nil
	}

	uniqueIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		uniqueIDs[id] = struct{}{}
	}

	result := make(map[string][]model.InstanceInfo, len(uniqueIDs))
	chunk := make([]interface{}, 0, 500)
	placeholders := make([]string, 0, 500)

	idList := make([]string, 0, len(uniqueIDs))
	for id := range uniqueIDs {
		idList = append(idList, id)
	}

	for i := 0; i < len(idList); i += 500 {
		chunk = chunk[:0]
		placeholders = placeholders[:0]
		end := i + 500
		if end > len(idList) {
			end = len(idList)
		}

		for _, id := range idList[i:end] {
			chunk = append(chunk, id)
			placeholders = append(placeholders, "?")
		}

		query := "SELECT id, instances FROM entities WHERE id IN (" + strings.Join(placeholders, ",") + ")"
		rows, err := tx.QueryContext(ctx, query, chunk...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id, instancesJSON string
			if err := rows.Scan(&id, &instancesJSON); err != nil {
				continue
			}

			var instances []model.InstanceInfo
			if err := json.Unmarshal([]byte(instancesJSON), &instances); err != nil {
				instances = []model.InstanceInfo{}
			}
			result[id] = instances
		}
	}

	return result, nil
}

// getEntityInstances retrieves existing instances for an entity.
func (s *SQLiteStore) getEntityInstances(ctx context.Context, tx *sql.Tx, entityID string) ([]model.InstanceInfo, error) {
	var instancesJSON string
	err := tx.QueryRowContext(ctx, "SELECT instances FROM entities WHERE id = ?", entityID).Scan(&instancesJSON)
	if err != nil {
		return nil, err
	}

	var instances []model.InstanceInfo
	if err := json.Unmarshal([]byte(instancesJSON), &instances); err != nil {
		return nil, err
	}
	return instances, nil
}

// mergeInstances merges new instances with existing ones, updating last_seen for duplicates.
func mergeInstances(existing, new []model.InstanceInfo) []model.InstanceInfo {
	instanceMap := make(map[string]*model.InstanceInfo)

	// Add existing instances
	for i := range existing {
		inst := &existing[i]
		instanceMap[inst.InstanceID] = inst
	}

	// Merge/update with new instances
	for _, inst := range new {
		if existing, ok := instanceMap[inst.InstanceID]; ok {
			// Update last seen and status
			existing.LastSeen = inst.LastSeen
			existing.Status = model.EntityStatusActive
			// Merge attrs
			for k, v := range inst.Attrs {
				existing.Attrs[k] = v
			}
		} else {
			// Add new instance
			instCopy := inst
			instanceMap[inst.InstanceID] = &instCopy
		}
	}

	// Convert back to slice
	result := make([]model.InstanceInfo, 0, len(instanceMap))
	for _, inst := range instanceMap {
		result = append(result, *inst)
	}
	return result
}

// UpsertRelations inserts or updates relations.
func (s *SQLiteStore) UpsertRelations(ctx context.Context, relations []*model.Relation) error {
	if len(relations) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Bulk fetch existing relations
	ids := make([]string, len(relations))
	for i, r := range relations {
		ids[i] = model.GenerateRelationID(r.FromID, r.ToID, r.Type)
	}

	existingSnapshots, err := s.getRelationsSnapshotBulk(ctx, tx, ids)
	if err != nil {
		s.logger.Error("Failed to bulk load relation snapshots", zap.Error(err))
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO relations (id, from_id, to_id, type, attrs, first_seen, last_seen, evidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(from_id, to_id, type) DO UPDATE SET
			first_seen = excluded.first_seen,
			attrs = excluded.attrs,
			last_seen = excluded.last_seen,
			evidence = excluded.evidence
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, relation := range relations {
		id := ids[i]

		// Merge with existing relation from map
		existing, found := existingSnapshots[id]
		
		existingAttrs := map[string]string{}
		existingEvidence := []model.Evidence{}
		existingFirst := relation.FirstSeen
		existingLast := relation.LastSeen
		isNew := !found

		if found {
			existingAttrs = existing.Attrs
			existingEvidence = existing.Evidence
			existingFirst = existing.FirstSeen
			existingLast = existing.LastSeen
		}

		incomingAttrs := relation.Attrs
		if incomingAttrs == nil {
			incomingAttrs = map[string]string{}
		}
		mergedAttrs, changedKeys := mergeAttrsWithChanges(existingAttrs, incomingAttrs)

		// For relations, keep evidence only when:
		// - relation is new, OR
		// - relation attrs were enriched (changedKeys non-empty).
		evidenceChangedKeys := changedKeys
		if !isNew && len(changedKeys) == 0 {
			evidenceChangedKeys = map[string]struct{}{} // no enrichment -> no new evidence
		}
		mergedEvidence := mergeEvidence(existingEvidence, relation.Evidence, evidenceChangedKeys, 80, 2)

		mergedFirstSeen := minTime(existingFirst, relation.FirstSeen)
		mergedLastSeen := maxTime(existingLast, relation.LastSeen)

		attrsJSON, err := json.Marshal(mergedAttrs)
		if err != nil {
			s.logger.Warn("Failed to marshal relation attrs", zap.Error(err))
			continue
		}

		evidenceJSON, err := json.Marshal(mergedEvidence)
		if err != nil {
			s.logger.Warn("Failed to marshal relation evidence", zap.Error(err))
			continue
		}

		_, err = stmt.ExecContext(ctx,
			id,
			relation.FromID,
			relation.ToID,
			string(relation.Type),
			string(attrsJSON),
			mergedFirstSeen,
			mergedLastSeen,
			string(evidenceJSON),
		)
		if err != nil {
			s.logger.Warn("Failed to upsert relation", zap.String("from", relation.FromID), zap.String("to", relation.ToID), zap.Error(err))
		}
	}

	return tx.Commit()
}

type relationSnapshot struct {
	Attrs     map[string]string
	Evidence  []model.Evidence
	FirstSeen time.Time
	LastSeen  time.Time
}

func (s *SQLiteStore) getRelationsSnapshotBulk(ctx context.Context, tx *sql.Tx, ids []string) (map[string]*relationSnapshot, error) {
	if len(ids) == 0 {
		return make(map[string]*relationSnapshot), nil
	}

	uniqueIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		uniqueIDs[id] = struct{}{}
	}

	result := make(map[string]*relationSnapshot, len(uniqueIDs))
	chunk := make([]interface{}, 0, 500)
	placeholders := make([]string, 0, 500)

	idList := make([]string, 0, len(uniqueIDs))
	for id := range uniqueIDs {
		idList = append(idList, id)
	}

	for i := 0; i < len(idList); i += 500 {
		chunk = chunk[:0]
		placeholders = placeholders[:0]
		end := i + 500
		if end > len(idList) {
			end = len(idList)
		}

		for _, id := range idList[i:end] {
			chunk = append(chunk, id)
			placeholders = append(placeholders, "?")
		}

		query := "SELECT id, attrs, evidence, first_seen, last_seen FROM relations WHERE id IN (" + strings.Join(placeholders, ",") + ")"
		rows, err := tx.QueryContext(ctx, query, chunk...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id string
			var attrsJSON sql.NullString
			var evidenceJSON string
			var firstSeen, lastSeen time.Time

			if err := rows.Scan(&id, &attrsJSON, &evidenceJSON, &firstSeen, &lastSeen); err != nil {
				continue
			}

			attrs := map[string]string{}
			if attrsJSON.Valid && strings.TrimSpace(attrsJSON.String) != "" {
				_ = json.Unmarshal([]byte(attrsJSON.String), &attrs)
			}

			var evidence []model.Evidence
			if strings.TrimSpace(evidenceJSON) != "" {
				if err := json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
					evidence = []model.Evidence{}
				}
			} else {
				evidence = []model.Evidence{}
			}

			result[id] = &relationSnapshot{
				Attrs:     attrs,
				Evidence:  evidence,
				FirstSeen: firstSeen,
				LastSeen:  lastSeen,
			}
		}
	}
	return result, nil
}

func (s *SQLiteStore) getRelationSnapshot(ctx context.Context, tx *sql.Tx, fromID, toID string, relType model.RelationType) (*relationSnapshot, error) {
	var attrsJSON sql.NullString
	var evidenceJSON string
	var firstSeen, lastSeen time.Time

	err := tx.QueryRowContext(ctx,
		`SELECT attrs, evidence, first_seen, last_seen FROM relations WHERE from_id = ? AND to_id = ? AND type = ?`,
		fromID, toID, string(relType),
	).Scan(&attrsJSON, &evidenceJSON, &firstSeen, &lastSeen)
	if err != nil {
		return nil, err
	}

	attrs := map[string]string{}
	if attrsJSON.Valid && strings.TrimSpace(attrsJSON.String) != "" {
		_ = json.Unmarshal([]byte(attrsJSON.String), &attrs)
	}

	var evidence []model.Evidence
	if strings.TrimSpace(evidenceJSON) != "" {
		if err := json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
			evidence = []model.Evidence{}
		}
	} else {
		evidence = []model.Evidence{}
	}

	return &relationSnapshot{
		Attrs:     attrs,
		Evidence:  evidence,
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}, nil
}

func minTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if b.After(a) {
		return b
	}
	return a
}

// GetEntity retrieves an entity by ID.
func (s *SQLiteStore) GetEntity(ctx context.Context, id string) (*model.Entity, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, type, name, attrs, first_seen, last_seen, evidence, status, instance_count, active_count, instances FROM entities WHERE id = ?",
		id,
	)

	return scanEntity(row)
}

// GetRelation retrieves a relation by from/to/type.
func (s *SQLiteStore) GetRelation(ctx context.Context, fromID, toID string, relType model.RelationType) (*model.Relation, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, from_id, to_id, type, attrs, first_seen, last_seen, evidence FROM relations WHERE from_id = ? AND to_id = ? AND type = ?",
		fromID, toID, string(relType),
	)

	return scanRelation(row)
}

// ListEntities lists entities with optional filtering.
func (s *SQLiteStore) ListEntities(ctx context.Context, filter EntityFilter) ([]*model.Entity, error) {
	query := "SELECT id, type, name, attrs, first_seen, last_seen, evidence, status, instance_count, active_count, instances FROM entities WHERE 1=1"
	args := []interface{}{}

	if len(filter.Types) > 0 {
		placeholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, st := range filter.Status {
			placeholders[i] = "?"
			args = append(args, string(st))
		}
		query += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}

	if filter.NamePattern != "" {
		query += " AND name LIKE ?"
		args = append(args, "%"+filter.NamePattern+"%")
	}

	query += " ORDER BY last_seen DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*model.Entity
	for rows.Next() {
		entity, err := scanEntityRows(rows)
		if err != nil {
			s.logger.Warn("Failed to scan entity", zap.Error(err))
			continue
		}
		entities = append(entities, entity)
	}

	return entities, rows.Err()
}

// CountEntities counts entities matching the filter (ignoring limit/offset).
func (s *SQLiteStore) CountEntities(ctx context.Context, filter EntityFilter) (int64, error) {
	query := "SELECT COUNT(*) FROM entities WHERE 1=1"
	args := []interface{}{}

	if len(filter.Types) > 0 {
		placeholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, st := range filter.Status {
			placeholders[i] = "?"
			args = append(args, string(st))
		}
		query += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}

	if filter.NamePattern != "" {
		query += " AND name LIKE ?"
		args = append(args, "%"+filter.NamePattern+"%")
	}

	var count int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// ListRelations lists relations with optional filtering.
func (s *SQLiteStore) ListRelations(ctx context.Context, filter RelationFilter) ([]*model.Relation, error) {
	query := "SELECT id, from_id, to_id, type, attrs, first_seen, last_seen, evidence FROM relations WHERE 1=1"
	args := []interface{}{}

	if len(filter.Types) > 0 {
		placeholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if filter.FromID != "" {
		query += " AND from_id = ?"
		args = append(args, filter.FromID)
	}

	if filter.ToID != "" {
		query += " AND to_id = ?"
		args = append(args, filter.ToID)
	}

	query += " ORDER BY last_seen DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relations []*model.Relation
	for rows.Next() {
		relation, err := scanRelationRows(rows)
		if err != nil {
			s.logger.Warn("Failed to scan relation", zap.Error(err))
			continue
		}
		relations = append(relations, relation)
	}

	return relations, rows.Err()
}

// CountRelations counts relations matching the filter (ignoring limit/offset).
func (s *SQLiteStore) CountRelations(ctx context.Context, filter RelationFilter) (int64, error) {
	query := "SELECT COUNT(*) FROM relations WHERE 1=1"
	args := []interface{}{}

	if len(filter.Types) > 0 {
		placeholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if filter.FromID != "" {
		query += " AND from_id = ?"
		args = append(args, filter.FromID)
	}

	if filter.ToID != "" {
		query += " AND to_id = ?"
		args = append(args, filter.ToID)
	}

	var count int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// GetEntityRelations gets all relations for an entity.
func (s *SQLiteStore) GetEntityRelations(ctx context.Context, entityID string, direction RelationDirection) ([]*model.Relation, error) {
	var query string
	var args []interface{}

	switch direction {
	case RelationDirectionIncoming:
		query = "SELECT id, from_id, to_id, type, attrs, first_seen, last_seen, evidence FROM relations WHERE to_id = ?"
		args = []interface{}{entityID}
	case RelationDirectionOutgoing:
		query = "SELECT id, from_id, to_id, type, attrs, first_seen, last_seen, evidence FROM relations WHERE from_id = ?"
		args = []interface{}{entityID}
	default:
		query = "SELECT id, from_id, to_id, type, attrs, first_seen, last_seen, evidence FROM relations WHERE from_id = ? OR to_id = ?"
		args = []interface{}{entityID, entityID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relations []*model.Relation
	for rows.Next() {
		relation, err := scanRelationRows(rows)
		if err != nil {
			s.logger.Warn("Failed to scan relation", zap.Error(err))
			continue
		}
		relations = append(relations, relation)
	}

	return relations, rows.Err()
}

// GetStats returns storage statistics.
func (s *SQLiteStore) GetStats(ctx context.Context) (*Stats, error) {
	stats := &Stats{
		EntityTypes:   make(map[model.EntityType]int64),
		RelationTypes: make(map[model.RelationType]int64),
	}

	// Get total entity count
	row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM entities")
	if err := row.Scan(&stats.EntityCount); err != nil {
		return nil, err
	}

	// Get total relation count
	row = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relations")
	if err := row.Scan(&stats.RelationCount); err != nil {
		return nil, err
	}

	// Get entity type counts
	rows, err := s.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM entities GROUP BY type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var entityType string
		var count int64
		if err := rows.Scan(&entityType, &count); err != nil {
			continue
		}
		stats.EntityTypes[model.EntityType(entityType)] = count
	}

	// Get relation type counts
	rows, err = s.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM relations GROUP BY type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var relType string
		var count int64
		if err := rows.Scan(&relType, &count); err != nil {
			continue
		}
		stats.RelationTypes[model.RelationType(relType)] = count
	}

	return stats, nil
}

// MarkStaleEntities marks entities as stale if not seen since the given threshold.
func (s *SQLiteStore) MarkStaleEntities(ctx context.Context, staleThreshold time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE entities SET status = ? WHERE status = ? AND last_seen < ?`,
		string(model.EntityStatusStale),
		string(model.EntityStatusActive),
		staleThreshold,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MarkInstancesStale marks instances as stale within entities if not seen since threshold.
func (s *SQLiteStore) MarkInstancesStale(ctx context.Context, staleThreshold time.Time) (int64, error) {
	// Get all entities with active instances
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, instances, active_count FROM entities WHERE active_count > 0`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var updatedCount int64
	for rows.Next() {
		var entityID, instancesJSON string
		var activeCount int
		if err := rows.Scan(&entityID, &instancesJSON, &activeCount); err != nil {
			continue
		}

		var instances []model.InstanceInfo
		if err := json.Unmarshal([]byte(instancesJSON), &instances); err != nil {
			continue
		}

		// Mark stale instances
		newActiveCount := 0
		modified := false
		for i := range instances {
			if instances[i].Status == model.EntityStatusActive {
				if instances[i].LastSeen.Before(staleThreshold) {
					instances[i].Status = model.EntityStatusStale
					modified = true
				} else {
					newActiveCount++
				}
			}
		}

		if modified {
			updatedJSON, err := json.Marshal(instances)
			if err != nil {
				continue
			}

			_, err = s.db.ExecContext(ctx,
				`UPDATE entities SET instances = ?, active_count = ? WHERE id = ?`,
				string(updatedJSON), newActiveCount, entityID,
			)
			if err != nil {
				s.logger.Warn("Failed to update stale instances", zap.String("id", entityID), zap.Error(err))
			} else {
				updatedCount++
			}
		}
	}

	return updatedCount, rows.Err()
}

// Close closes the store.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Scanner interface for scanning rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanEntity(s scanner) (*model.Entity, error) {
	var entity model.Entity
	var entityType, attrsJSON, evidenceJSON string
	var status sql.NullString
	var instanceCount, activeCount sql.NullInt64
	var instancesJSON sql.NullString
	var firstSeen, lastSeen time.Time

	err := s.Scan(
		&entity.ID,
		&entityType,
		&entity.Name,
		&attrsJSON,
		&firstSeen,
		&lastSeen,
		&evidenceJSON,
		&status,
		&instanceCount,
		&activeCount,
		&instancesJSON,
	)
	if err != nil {
		return nil, err
	}

	entity.Type = model.EntityType(entityType)
	entity.FirstSeen = firstSeen
	entity.LastSeen = lastSeen

	if err := json.Unmarshal([]byte(attrsJSON), &entity.Attrs); err != nil {
		entity.Attrs = make(map[string]string)
	}

	if err := json.Unmarshal([]byte(evidenceJSON), &entity.Evidence); err != nil {
		entity.Evidence = []model.Evidence{}
	}

	// Parse new fields
	if status.Valid && status.String != "" {
		entity.Status = model.EntityStatus(status.String)
	} else {
		entity.Status = model.EntityStatusActive
	}

	if instanceCount.Valid {
		entity.InstanceCount = int(instanceCount.Int64)
	}

	if activeCount.Valid {
		entity.ActiveCount = int(activeCount.Int64)
	}

	if instancesJSON.Valid && instancesJSON.String != "" {
		if err := json.Unmarshal([]byte(instancesJSON.String), &entity.Instances); err != nil {
			entity.Instances = []model.InstanceInfo{}
		}
	}

	return &entity, nil
}

func scanEntityRows(rows *sql.Rows) (*model.Entity, error) {
	return scanEntity(rows)
}

func scanRelation(s scanner) (*model.Relation, error) {
	var relation model.Relation
	var id, relType string
	var attrsJSON, evidenceJSON sql.NullString
	var firstSeen, lastSeen time.Time

	err := s.Scan(
		&id,
		&relation.FromID,
		&relation.ToID,
		&relType,
		&attrsJSON,
		&firstSeen,
		&lastSeen,
		&evidenceJSON,
	)
	if err != nil {
		return nil, err
	}

	relation.Type = model.RelationType(relType)
	relation.FirstSeen = firstSeen
	relation.LastSeen = lastSeen

	if attrsJSON.Valid && attrsJSON.String != "" {
		if err := json.Unmarshal([]byte(attrsJSON.String), &relation.Attrs); err != nil {
			relation.Attrs = make(map[string]string)
		}
	}

	if evidenceJSON.Valid && evidenceJSON.String != "" {
		if err := json.Unmarshal([]byte(evidenceJSON.String), &relation.Evidence); err != nil {
			relation.Evidence = []model.Evidence{}
		}
	}

	return &relation, nil
}

func scanRelationRows(rows *sql.Rows) (*model.Relation, error) {
	return scanRelation(rows)
}
