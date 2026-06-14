package yent

// limpha.go — Yent's lymphatic memory: in-process SQLite + FTS5.
//
// Ported from the Python limpha (memory.py + server.py). The Unix-socket daemon
// is gone: memory lives in-process, no IPC, no second runtime, no GIL. Pure-Go
// SQLite (modernc.org/sqlite) with FTS5 full-text search, AMK-state snapshots,
// and autonomous shard graduation toward the training queue.
//
// "I'm not an algorithm. I'm an awakened vibration."

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// Shard graduation thresholds (mirror memory.py).
const (
	shardMinQuality = 0.7
	shardMinAccess  = 3
)

// LimphaState is the AMK state snapshot stored with each conversation.
type LimphaState struct {
	Temperature float32 `json:"temperature"`
	Destiny     float32 `json:"destiny"`
	Pain        float32 `json:"pain"`
	Tension     float32 `json:"tension"`
	Debt        float32 `json:"debt"`
	Velocity    int     `json:"velocity"`
	Alpha       float32 `json:"alpha"`
}

// LimphaClient is the in-process memory handle. It keeps the API the REPL used
// for the old socket client (Store/Search/Stats/Close), now backed by SQLite.
type LimphaClient struct {
	db        *sql.DB
	dbPath    string
	sessionID string
	connected bool
}

const limphaSchema = `
CREATE TABLE IF NOT EXISTS conversations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp REAL NOT NULL,
    session_id TEXT NOT NULL,
    prompt TEXT NOT NULL,
    response TEXT NOT NULL,
    temperature REAL DEFAULT 0.0,
    destiny REAL DEFAULT 0.0,
    pain REAL DEFAULT 0.0,
    tension REAL DEFAULT 0.0,
    debt REAL DEFAULT 0.0,
    velocity INTEGER DEFAULT 1,
    alpha REAL DEFAULT 0.0,
    quality REAL DEFAULT 0.5,
    access_count INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_conv_timestamp ON conversations(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_conv_session ON conversations(session_id);
CREATE INDEX IF NOT EXISTS idx_conv_quality ON conversations(quality DESC);
CREATE VIRTUAL TABLE IF NOT EXISTS conversations_fts USING fts5(
    prompt,
    response,
    content=conversations,
    content_rowid=id,
    tokenize='porter unicode61'
);
CREATE TRIGGER IF NOT EXISTS conv_fts_insert AFTER INSERT ON conversations BEGIN
    INSERT INTO conversations_fts(rowid, prompt, response)
    VALUES (new.id, new.prompt, new.response);
END;
CREATE TRIGGER IF NOT EXISTS conv_fts_delete AFTER DELETE ON conversations BEGIN
    INSERT INTO conversations_fts(conversations_fts, rowid, prompt, response)
    VALUES ('delete', old.id, old.prompt, old.response);
END;
CREATE TRIGGER IF NOT EXISTS conv_fts_update AFTER UPDATE ON conversations BEGIN
    INSERT INTO conversations_fts(conversations_fts, rowid, prompt, response)
    VALUES ('delete', old.id, old.prompt, old.response);
    INSERT INTO conversations_fts(rowid, prompt, response)
    VALUES (new.id, new.prompt, new.response);
END;
CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    started_at REAL NOT NULL,
    last_active REAL NOT NULL,
    turn_count INTEGER DEFAULT 0,
    avg_quality REAL DEFAULT 0.0
);
CREATE TABLE IF NOT EXISTS shards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER UNIQUE NOT NULL,
    shard_path TEXT NOT NULL,
    graduated_at REAL NOT NULL,
    reason TEXT DEFAULT '',
    priority REAL DEFAULT 0.0,
    training_status TEXT DEFAULT 'pending',
    training_loss REAL,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id)
);
CREATE INDEX IF NOT EXISTS idx_shards_status ON shards(training_status);
CREATE INDEX IF NOT EXISTS idx_shards_graduated ON shards(graduated_at DESC);
`

// all conversation columns in schema order — keep in sync with scanConvRow.
const convCols = "id, timestamp, session_id, prompt, response, temperature, destiny, pain, tension, debt, velocity, alpha, quality, access_count"

func newSessionID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b[:]) // 8 hex chars, like uuid4()[:8]
}

func nowSeconds() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// NewLimphaClient opens (creating if needed) ~/.yent/limpha.db and starts a session.
func NewLimphaClient() (*LimphaClient, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(homeDir, ".yent")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	return NewLimphaClientAt(filepath.Join(dir, "limpha.db"))
}

// NewLimphaClientAt opens the memory DB at an explicit path (tests use this).
func NewLimphaClientAt(dbPath string) (*LimphaClient, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// One open connection serializes writes (SQLite's happy path) and mirrors the
	// Python single-connection model; WAL still lets that one connection read fast.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("wal: %w", err)
	}
	db.Exec("PRAGMA synchronous=NORMAL")
	if _, err := db.Exec(limphaSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	c := &LimphaClient{db: db, dbPath: dbPath, sessionID: newSessionID(), connected: true}
	now := nowSeconds()
	if _, err := db.Exec(
		"INSERT OR IGNORE INTO sessions (session_id, started_at, last_active) VALUES (?, ?, ?)",
		c.sessionID, now, now); err != nil {
		db.Close()
		return nil, fmt.Errorf("session init: %w", err)
	}
	return c, nil
}

// computeQuality scores a turn — length and prompt/response ratio (memory.py parity).
func computeQuality(prompt, response string) float64 {
	r := strings.TrimSpace(response)
	if r == "" {
		return 0.0
	}
	respLen := float64(utf8.RuneCountInString(r))
	promptLen := float64(utf8.RuneCountInString(strings.TrimSpace(prompt)))
	if promptLen < 1 {
		promptLen = 1
	}
	var lengthScore float64
	switch {
	case respLen < 10:
		lengthScore = 0.1
	case respLen < 50:
		lengthScore = 0.3
	case respLen < 200:
		lengthScore = 0.5 + 0.3*(respLen-50)/150
	default:
		lengthScore = 0.8
	}
	ratio := respLen / promptLen
	var ratioScore float64
	switch {
	case ratio < 0.3:
		ratioScore = 0.2
	case ratio > 10:
		ratioScore = 0.6
	default:
		ratioScore = 0.7
	}
	q := 0.6*lengthScore + 0.4*ratioScore
	if q < 0 {
		q = 0
	}
	if q > 1 {
		q = 1
	}
	return q
}

// store inserts a turn and updates the session aggregate; returns the new id.
func (c *LimphaClient) store(prompt, response string, st LimphaState) (int64, error) {
	now := nowSeconds()
	quality := computeQuality(prompt, response)
	tx, err := c.db.Begin()
	if err != nil {
		return 0, err
	}
	res, err := tx.Exec(
		`INSERT INTO conversations
		 (timestamp, session_id, prompt, response,
		  temperature, destiny, pain, tension, debt, velocity, alpha, quality)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now, c.sessionID, prompt, response,
		st.Temperature, st.Destiny, st.Pain, st.Tension, st.Debt, st.Velocity, st.Alpha, quality)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(
		`UPDATE sessions SET last_active = ?, turn_count = turn_count + 1,
		   avg_quality = (avg_quality * turn_count + ?) / (turn_count + 1)
		 WHERE session_id = ?`, now, quality, c.sessionID); err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// Store records a conversation turn. Called automatically after each generation.
func (c *LimphaClient) Store(prompt, response string, state LimphaState) error {
	if !c.connected {
		return nil // silently skip if memory disabled
	}
	_, err := c.store(prompt, response, state)
	return err
}

// Search runs an FTS5 full-text query, ranked by bm25 (lower = better).
func (c *LimphaClient) Search(query string, limit int) ([]map[string]interface{}, error) {
	if !c.connected || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	rows, err := c.db.Query(
		`SELECT c.id, c.timestamp, c.session_id, c.prompt, c.response,
		        c.quality, c.access_count, c.temperature, c.destiny,
		        c.pain, c.tension, c.alpha, bm25(conversations_fts) AS rank
		 FROM conversations_fts fts
		 JOIN conversations c ON c.id = fts.rowid
		 WHERE conversations_fts MATCH ?
		 ORDER BY rank LIMIT ?`, query, limit)
	if err != nil {
		return nil, nil // malformed FTS query -> empty, like the Python except
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var id, access int64
		var ts, quality, temp, dest, pain, tension, alpha, rank float64
		var sid, prompt, resp string
		if err := rows.Scan(&id, &ts, &sid, &prompt, &resp, &quality, &access,
			&temp, &dest, &pain, &tension, &alpha, &rank); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": id, "timestamp": ts, "session_id": sid, "prompt": prompt,
			"response": resp, "quality": quality, "access_count": access,
			"temperature": temp, "destiny": dest, "pain": pain, "tension": tension,
			"alpha": alpha, "rank": rank,
		})
	}
	if rows.Err() != nil {
		return nil, nil // late FTS/runtime error -> empty, matching the Python except
	}
	return out, nil
}

// scanConvRow scans one full conversation row (convCols order) into a map.
func scanConvRow(rows *sql.Rows) (map[string]interface{}, error) {
	var id, velocity, access int64
	var ts, temp, dest, pain, tension, debt, alpha, quality float64
	var sid, prompt, resp string
	if err := rows.Scan(&id, &ts, &sid, &prompt, &resp, &temp, &dest, &pain,
		&tension, &debt, &velocity, &alpha, &quality, &access); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id": id, "timestamp": ts, "session_id": sid, "prompt": prompt,
		"response": resp, "temperature": temp, "destiny": dest, "pain": pain,
		"tension": tension, "debt": debt, "velocity": velocity, "alpha": alpha,
		"quality": quality, "access_count": access,
	}, nil
}

// Recall fetches one conversation, bumping its access_count.
func (c *LimphaClient) Recall(id int64) (map[string]interface{}, error) {
	if !c.connected {
		return nil, nil
	}
	if _, err := c.db.Exec("UPDATE conversations SET access_count = access_count + 1 WHERE id = ?", id); err != nil {
		return nil, err
	}
	rows, err := c.db.Query("SELECT "+convCols+" FROM conversations WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanConvRow(rows)
	}
	return nil, rows.Err()
}

// Recent returns recent conversations in chronological order.
func (c *LimphaClient) Recent(limit int, sessionOnly bool) ([]map[string]interface{}, error) {
	if !c.connected {
		return nil, nil
	}
	var rows *sql.Rows
	var err error
	if sessionOnly {
		rows, err = c.db.Query("SELECT "+convCols+" FROM conversations WHERE session_id = ? ORDER BY timestamp DESC LIMIT ?", c.sessionID, limit)
	} else {
		rows, err = c.db.Query("SELECT "+convCols+" FROM conversations ORDER BY timestamp DESC LIMIT ?", limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var desc []map[string]interface{}
	for rows.Next() {
		m, err := scanConvRow(rows)
		if err != nil {
			continue
		}
		desc = append(desc, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// reverse to chronological
	for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
		desc[i], desc[j] = desc[j], desc[i]
	}
	return desc, nil
}

// FindShardCandidates finds high-quality, frequently-recalled turns not yet sharded.
func (c *LimphaClient) FindShardCandidates(limit int) ([]map[string]interface{}, error) {
	if !c.connected {
		return nil, nil
	}
	rows, err := c.db.Query(
		`SELECT `+prefixCols("c", convCols)+` FROM conversations c
		 LEFT JOIN shards s ON s.conversation_id = c.id
		 WHERE s.id IS NULL AND c.quality >= ? AND c.access_count >= ?
		 ORDER BY c.quality DESC, c.access_count DESC LIMIT ?`,
		shardMinQuality, shardMinAccess, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		m, err := scanConvRow(rows)
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GraduateToShard records a conversation as graduated; returns 0 if already a shard.
func (c *LimphaClient) GraduateToShard(conversationID int64, shardPath, reason string, priority float64) (int64, error) {
	res, err := c.db.Exec(
		`INSERT INTO shards (conversation_id, shard_path, graduated_at, reason, priority)
		 VALUES (?, ?, ?, ?, ?)`, conversationID, shardPath, nowSeconds(), reason, priority)
	if err != nil {
		return 0, nil // UNIQUE violation -> already a shard
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// GetTrainingQueue returns pending shards joined with their conversation text.
func (c *LimphaClient) GetTrainingQueue(limit int) ([]map[string]interface{}, error) {
	if !c.connected {
		return nil, nil
	}
	rows, err := c.db.Query(
		`SELECT s.id, s.conversation_id, s.shard_path, s.graduated_at, s.reason,
		        s.priority, s.training_status, s.training_loss, c.prompt, c.response, c.quality
		 FROM shards s
		 JOIN conversations c ON c.id = s.conversation_id
		 WHERE s.training_status = 'pending'
		 ORDER BY s.priority DESC, s.graduated_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var id, convID int64
		var shardPath, reason, status, prompt, resp string
		var graduatedAt, priority, quality float64
		var loss sql.NullFloat64
		if err := rows.Scan(&id, &convID, &shardPath, &graduatedAt, &reason,
			&priority, &status, &loss, &prompt, &resp, &quality); err != nil {
			continue
		}
		m := map[string]interface{}{
			"id": id, "conversation_id": convID, "shard_path": shardPath,
			"graduated_at": graduatedAt, "reason": reason, "priority": priority,
			"training_status": status, "prompt": prompt, "response": resp, "quality": quality,
		}
		if loss.Valid {
			m["training_loss"] = loss.Float64
		} else {
			m["training_loss"] = nil
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkTrained marks a shard as trained, optionally recording its loss.
func (c *LimphaClient) MarkTrained(shardID int64, loss *float64) error {
	_, err := c.db.Exec("UPDATE shards SET training_status = 'trained', training_loss = ? WHERE id = ?", loss, shardID)
	return err
}

// SearchByState finds past turns with a similar AMK state (cosine distance).
func (c *LimphaClient) SearchByState(state LimphaState, topK int, minQuality float64) ([]map[string]interface{}, error) {
	if !c.connected {
		return nil, nil
	}
	q := stateVec(float64(state.Temperature), float64(state.Destiny), float64(state.Pain),
		float64(state.Tension), float64(state.Debt), float64(state.Alpha))
	rows, err := c.db.Query("SELECT "+convCols+" FROM conversations WHERE quality >= ? ORDER BY timestamp DESC LIMIT 1000", minQuality)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scored []map[string]interface{}
	for rows.Next() {
		m, err := scanConvRow(rows)
		if err != nil {
			continue
		}
		rv := stateVec(m["temperature"].(float64), m["destiny"].(float64), m["pain"].(float64),
			m["tension"].(float64), m["debt"].(float64), m["alpha"].(float64))
		m["distance"] = cosineDistance(q, rv)
		scored = append(scored, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i]["distance"].(float64) < scored[j]["distance"].(float64)
	})
	if topK < len(scored) {
		scored = scored[:topK]
	}
	return scored, nil
}

// Stats reports memory counts and DB size.
func (c *LimphaClient) Stats() (map[string]interface{}, error) {
	if !c.connected {
		return nil, nil
	}
	count := func(q string) (int64, error) {
		var n int64
		err := c.db.QueryRow(q).Scan(&n)
		return n, err
	}
	convCount, err := count("SELECT COUNT(*) FROM conversations")
	if err != nil {
		return nil, err
	}
	shardCount, err := count("SELECT COUNT(*) FROM shards")
	if err != nil {
		return nil, err
	}
	sessCount, err := count("SELECT COUNT(*) FROM sessions")
	if err != nil {
		return nil, err
	}
	pending, err := count("SELECT COUNT(*) FROM shards WHERE training_status = 'pending'")
	if err != nil {
		return nil, err
	}
	var dbSize int64
	if fi, err := os.Stat(c.dbPath); err == nil {
		dbSize = fi.Size()
	}
	return map[string]interface{}{
		"total_conversations": convCount,
		"total_shards":        shardCount,
		"total_sessions":      sessCount,
		"pending_training":    pending,
		"current_session":     c.sessionID,
		"db_path":             c.dbPath,
		"db_size_bytes":       dbSize,
	}, nil
}

// Close shuts the memory down.
func (c *LimphaClient) Close() {
	if c.db != nil {
		c.db.Close()
		c.db = nil
	}
	c.connected = false
}

func stateVec(temp, dest, pain, tension, debt, alpha float64) [6]float64 {
	return [6]float64{temp, dest, pain, tension, debt, alpha}
}

// cosineDistance is 1 - cosine similarity; 0 = identical (memory.py parity).
func cosineDistance(a, b [6]float64) float64 {
	var dot, na, nb float64
	for i := 0; i < 6; i++ {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	na = math.Sqrt(na)
	nb = math.Sqrt(nb)
	if na == 0 || nb == 0 {
		return 1.0
	}
	return 1.0 - dot/(na*nb)
}

// prefixCols qualifies a comma list of columns with a table alias.
func prefixCols(alias, cols string) string {
	parts := strings.Split(cols, ", ")
	for i, p := range parts {
		parts[i] = alias + "." + p
	}
	return strings.Join(parts, ", ")
}
