package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

type ScoreEntry struct {
	ID          int64   `json:"id"`
	Rank        int     `json:"rank"`
	Level       string  `json:"level"`
	League      string  `json:"league"`
	UserID      string  `json:"userId"`
	Username    string  `json:"username"`
	TimeTaken   float64 `json:"timeTaken"`
	Clicks      int     `json:"clicks"`
	Score       float64 `json:"score"`
	Difficulty  string  `json:"difficulty"`
	Status      int     `json:"status"` // 0 = Lose, 1 = Win
	Checkpoints int     `json:"checkpoints"`
	SubmittedAt string  `json:"submittedAt"`
}

type LevelInfo struct {
	Name        string  `json:"name"`
	TotalScores int     `json:"totalScores"`
	BestTime    float64 `json:"bestTime"`
	TopPlayer   string  `json:"topPlayer"`
	TopScore    float64 `json:"topScore"`
}

type LeagueInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Levels       []string `json:"levels"`
	IsLocked     bool     `json:"isLocked"`
	TotalPlayers int      `json:"totalPlayers"`
	TotalLevels  int      `json:"totalLevels"`
	TopPlayer    string   `json:"topPlayer"`
	TopScore     float64  `json:"topScore"`
}

type LeagueItem struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Levels      []string `json:"levels"`
	IsLocked    bool     `json:"isLocked,omitempty"`
}

type LeagueEntry struct {
	Rank          int     `json:"rank"`
	League        string  `json:"league"`
	UserID        string  `json:"userId"`
	Username      string  `json:"username"`
	TotalScore    float64 `json:"totalScore"`
	LevelsCleared int     `json:"levelsCleared"`
	TotalTime     float64 `json:"totalTime"`
	TotalClicks   int     `json:"totalClicks"`
	LastActive    string  `json:"lastActive"`
}

var db *sql.DB

func initDB(dataSourceName string) (*sql.DB, error) {
	var err error
	db, err = sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Base tables
	schema := `
	CREATE TABLE IF NOT EXISTS level_leaderboards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		level TEXT NOT NULL,
		user_id TEXT NOT NULL,
		username TEXT NOT NULL,
		time_taken REAL NOT NULL,
		clicks INTEGER NOT NULL,
		status TEXT NOT NULL,
		checkpoints INTEGER DEFAULT 0,
		submitted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(level, user_id)
	);

	CREATE TABLE IF NOT EXISTS leagues (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		levels TEXT DEFAULT '',
		is_locked INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS level_stats (
		level TEXT PRIMARY KEY,
		min_time_ms INTEGER DEFAULT 0,
		max_time_ms INTEGER DEFAULT 0,
		max_checkpoints INTEGER DEFAULT 0,
		total_runs INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_leaderboards_level ON level_leaderboards(level);
	CREATE INDEX IF NOT EXISTS idx_leaderboards_rank ON level_leaderboards(level, time_taken ASC, clicks ASC);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create base schema: %w", err)
	}

	// Migrate missing columns if level_leaderboards already existed
	migrateColumns()

	// Backfill any zero scores with calculated values
	recalculateAllScores()

	log.Println("Database initialized successfully with leagues and level_leaderboards.")
	return db, nil
}

func migrateColumns() {
	rows, err := db.Query("PRAGMA table_info(level_leaderboards)")
	if err != nil {
		log.Printf("Warning: failed to check columns: %v", err)
		return
	}
	defer rows.Close()

	hasScore := false
	hasDiff := false
	hasLeague := false

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
			switch name {
			case "score":
				hasScore = true
			case "difficulty":
				hasDiff = true
			case "league":
				hasLeague = true
			}
		}
	}

	if !hasScore {
		_, _ = db.Exec("ALTER TABLE level_leaderboards ADD COLUMN score REAL DEFAULT 0")
	}
	if !hasDiff {
		_, _ = db.Exec("ALTER TABLE level_leaderboards ADD COLUMN difficulty TEXT DEFAULT '0.25'")
	}
	if !hasLeague {
		_, _ = db.Exec("ALTER TABLE level_leaderboards ADD COLUMN league TEXT DEFAULT ''")
	}

	// Normalize status to integer: 1 for Win, 0 for Lose
	_, _ = db.Exec("UPDATE level_leaderboards SET status = 1 WHERE status = 'Win!' OR status = 'win' OR status = '1' OR status = 1")
	_, _ = db.Exec("UPDATE level_leaderboards SET status = 0 WHERE status != 1 AND status != '1'")

	// Normalize difficulty to numeric representation ("0.25", "0.50", "0.75", "1.00")
	_, _ = db.Exec("UPDATE level_leaderboards SET difficulty = '0.25' WHERE difficulty = 'easy'")
	_, _ = db.Exec("UPDATE level_leaderboards SET difficulty = '0.50' WHERE difficulty IN ('medium', 'normal')")
	_, _ = db.Exec("UPDATE level_leaderboards SET difficulty = '0.75' WHERE difficulty = 'hard'")
	_, _ = db.Exec("UPDATE level_leaderboards SET difficulty = '1.00' WHERE difficulty IN ('expert', 'insane')")

	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_leaderboards_league ON level_leaderboards(league)")

	// Migrate leagues table for is_locked and levels columns
	rowsL, err := db.Query("PRAGMA table_info(leagues)")
	if err == nil {
		defer rowsL.Close()
		hasLocked := false
		hasLevels := false
		for rowsL.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dfltValue sql.NullString
			if err := rowsL.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
				if name == "is_locked" {
					hasLocked = true
				} else if name == "levels" {
					hasLevels = true
				}
			}
		}
		if !hasLocked {
			_, _ = db.Exec("ALTER TABLE leagues ADD COLUMN is_locked INTEGER DEFAULT 0")
		}
		if !hasLevels {
			_, _ = db.Exec("ALTER TABLE leagues ADD COLUMN levels TEXT DEFAULT ''")
		}
	}
}

type LevelStats struct {
	Level          string `json:"level"`
	MinTimeMs      int64  `json:"minTimeMs"`
	MaxTimeMs      int64  `json:"maxTimeMs"`
	MaxCheckpoints int    `json:"maxCheckpoints"`
	TotalRuns      int    `json:"totalRuns"`
}

func recalculateAllScores() {
	rows, err := db.Query(`SELECT DISTINCT level FROM level_leaderboards`)
	if err != nil {
		return
	}
	defer rows.Close()

	var levels []string
	for rows.Next() {
		var lvl string
		if err := rows.Scan(&lvl); err == nil {
			levels = append(levels, lvl)
		}
	}

	for _, lvl := range levels {
		var maxChk int
		_ = db.QueryRow(`SELECT COALESCE(MAX(checkpoints), 0) FROM level_leaderboards WHERE level = ?`, lvl).Scan(&maxChk)

		var minTime float64
		var maxTime float64
		var count int
		_ = db.QueryRow(`
			SELECT COALESCE(MIN(time_taken), 0), COALESCE(MAX(time_taken), 0), COUNT(*)
			FROM level_leaderboards
			WHERE level = ? AND checkpoints >= ? AND checkpoints > 0`, lvl, maxChk).Scan(&minTime, &maxTime, &count)

		if count > 0 {
			minMs := int64(math.Round(minTime * 1000.0))
			maxMs := int64(math.Round(maxTime * 1000.0))
			if maxMs > 900000 {
				maxMs = 900000
			}
			_, _ = db.Exec(`
				INSERT INTO level_stats (level, min_time_ms, max_time_ms, max_checkpoints, total_runs, updated_at)
				VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
				ON CONFLICT(level) DO UPDATE SET
					min_time_ms = excluded.min_time_ms,
					max_time_ms = excluded.max_time_ms,
					max_checkpoints = excluded.max_checkpoints,
					total_runs = excluded.total_runs,
					updated_at = CURRENT_TIMESTAMP`,
				lvl, minMs, maxMs, maxChk, count)
		}
	}
}

func clampMultiplier(v float64) float64 {
	if v < 0.0 {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

// ParseDifficultyMultiplier resolves difficulty to a multiplier strictly in the range [0.0, 1.0]
func ParseDifficultyMultiplier(diffVal interface{}) (float64, string) {
	if diffVal == nil {
		return 0.25, "0.25"
	}
	switch v := diffVal.(type) {
	case float64:
		mult := clampMultiplier(v)
		return mult, fmt.Sprintf("%.2f", mult)
	case float32:
		mult := clampMultiplier(float64(v))
		return mult, fmt.Sprintf("%.2f", mult)
	case int:
		mult := clampMultiplier(float64(v))
		return mult, fmt.Sprintf("%.2f", mult)
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		switch lower {
		case "easy":
			return 0.25, "0.25"
		case "medium", "normal":
			return 0.50, "0.50"
		case "hard":
			return 0.75, "0.75"
		case "expert", "insane":
			return 1.00, "1.00"
		default:
			if parsed, err := strconv.ParseFloat(lower, 64); err == nil {
				mult := clampMultiplier(parsed)
				return mult, fmt.Sprintf("%.2f", mult)
			}
		}
	}
	return 0.25, "0.25"
}

// UpdateAndCalculateScore updates level min/max stats and computes the 1,100-point score:
//   Primary: checkpoints completed (raw count)
//   Secondary: time in milliseconds
//   Tertiary: clicks (optional, multiplied by 0)
//
// Ratio:
//   Checkpoint Score = RankPct(checkpoint) * 1000.0   (1000 max pts)
//   Time Score       = RankPct(time) * 100.0          (100 max pts)
//   Tertiary Score   = tertiary * 0.0 = 0.0
//   Base Score       = round(Checkpoint Score + Time Score) [Max = 1,100 pts]
//   If checkpoints <= 0: Base Score = 0 (early quit gets 0 pts)
func UpdateAndCalculateScore(level string, checkpoints int, timeMs int64, tertiary float64, diffVal interface{}) (float64, string, error) {
	_, diffName := ParseDifficultyMultiplier(diffVal)

	if checkpoints <= 0 {
		return 0.0, diffName, nil
	}

	if timeMs < 0 {
		timeMs = 0
	}
	// Cap time at 15 minutes (900,000 ms) so AFK runs cannot break the scale
	cappedTimeMs := timeMs
	if cappedTimeMs > 900000 {
		cappedTimeMs = 900000
	}

	var stats LevelStats
	stats.Level = level

	err := db.QueryRow(`
		SELECT min_time_ms, max_time_ms, max_checkpoints, total_runs 
		FROM level_stats 
		WHERE level = ?`, level).Scan(&stats.MinTimeMs, &stats.MaxTimeMs, &stats.MaxCheckpoints, &stats.TotalRuns)

	if err == sql.ErrNoRows {
		// First player for this level
		stats.MinTimeMs = cappedTimeMs
		stats.MaxTimeMs = cappedTimeMs
		stats.MaxCheckpoints = checkpoints
		stats.TotalRuns = 1

		_, _ = db.Exec(`
			INSERT INTO level_stats (level, min_time_ms, max_time_ms, max_checkpoints, total_runs, updated_at)
			VALUES (?, ?, ?, ?, 1, CURRENT_TIMESTAMP)`,
			level, stats.MinTimeMs, stats.MaxTimeMs, stats.MaxCheckpoints)

		// First player gets full 1,100 points
		baseScore := 1000.0 + 100.0
		return math.Round(baseScore), diffName, nil
	} else if err != nil {
		return 0, diffName, err
	}

	// Update running stats with current run
	if checkpoints > stats.MaxCheckpoints {
		stats.MaxCheckpoints = checkpoints
		// When a new depth record is set, this run's time becomes the baseline min_time for full completion
		stats.MinTimeMs = cappedTimeMs
	} else if checkpoints >= stats.MaxCheckpoints {
		// ONLY qualifying runs (reaching the level's max checkpoints) can set a new fastest time
		if stats.MinTimeMs == 0 || cappedTimeMs < stats.MinTimeMs {
			stats.MinTimeMs = cappedTimeMs
		}
	}

	if cappedTimeMs > stats.MaxTimeMs {
		stats.MaxTimeMs = cappedTimeMs
	}
	stats.TotalRuns++

	_, _ = db.Exec(`
		UPDATE level_stats 
		SET min_time_ms = ?, max_time_ms = ?, max_checkpoints = ?, total_runs = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE level = ?`,
		stats.MinTimeMs, stats.MaxTimeMs, stats.MaxCheckpoints, stats.TotalRuns, level)

	// Calculate Rank Percentages
	chkPct := 1.0
	if stats.MaxCheckpoints > 0 {
		chkPct = float64(checkpoints) / float64(stats.MaxCheckpoints)
		if chkPct > 1.0 {
			chkPct = 1.0
		}
	}

	timePct := 1.0
	if stats.MaxTimeMs > stats.MinTimeMs {
		timePct = float64(stats.MaxTimeMs-cappedTimeMs) / float64(stats.MaxTimeMs-stats.MinTimeMs)
		if timePct < 0.0 {
			timePct = 0.0
		}
		if timePct > 1.0 {
			timePct = 1.0
		}
	}

	chkScore := chkPct * 1000.0
	timeScore := timePct * 100.0
	tertScore := tertiary * 0.0

	baseScore := chkScore + timeScore + tertScore
	return math.Round(baseScore), diffName, nil
}

// CalculateScore computes the 1,100-point score using rank percentages directly:
//   Primary: checkpoint rank percentage (0.0 to 1.0)
//   Secondary: time rank percentage (0.0 to 1.0)
//   Tertiary: raw clicks (optional, multiplied by 0)
func CalculateScore(primary float64, secondary float64, tertiary float64, diffVal interface{}) (float64, string) {
	_, diffName := ParseDifficultyMultiplier(diffVal)
	if primary <= 0.0 {
		return 0.0, diffName
	}
	if primary > 1.0 {
		primary = primary / 100.0
		if primary > 1.0 {
			primary = 1.0
		}
	}
	if secondary < 0.0 {
		secondary = 0.0
	}
	if secondary > 1.0 {
		secondary = secondary / 100.0
		if secondary > 1.0 {
			secondary = 1.0
		}
	}
	base := (primary * 1000.0) + (secondary * 100.0) + (tertiary * 0.0)
	return math.Round(base), diffName
}

// SaveScore records or updates a player's score for a level and league.
func SaveScore(entry ScoreEntry) (int, bool, error) {
	if entry.Difficulty == "" {
		entry.Difficulty = "easy"
	}

	var existingScore float64
	var existingTime float64
	var isNew bool

	err := db.QueryRow(`
		SELECT score, time_taken 
		FROM level_leaderboards 
		WHERE level = ? AND user_id = ?`,
		entry.Level, entry.UserID).Scan(&existingScore, &existingTime)

	if err == sql.ErrNoRows {
		// New entry for this player on this level
		isNew = true
		_, err = db.Exec(`
			INSERT INTO level_leaderboards (level, league, user_id, username, time_taken, clicks, score, difficulty, status, checkpoints, submitted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			entry.Level, entry.League, entry.UserID, entry.Username, entry.TimeTaken, entry.Clicks, entry.Score, entry.Difficulty, entry.Status, entry.Checkpoints)
		if err != nil {
			return 0, false, fmt.Errorf("failed to insert new score: %w", err)
		}
	} else if err != nil {
		return 0, false, fmt.Errorf("failed to check existing score: %w", err)
	} else {
		// Update if new score is higher, or if score is equal and time is faster
		isBetter := entry.Score > existingScore || (entry.Score == existingScore && entry.TimeTaken < existingTime)
		if isBetter {
			_, err = db.Exec(`
				UPDATE level_leaderboards 
				SET league = ?, username = ?, time_taken = ?, clicks = ?, score = ?, difficulty = ?, status = ?, checkpoints = ?, submitted_at = CURRENT_TIMESTAMP
				WHERE level = ? AND user_id = ?`,
				entry.League, entry.Username, entry.TimeTaken, entry.Clicks, entry.Score, entry.Difficulty, entry.Status, entry.Checkpoints, entry.Level, entry.UserID)
			if err != nil {
				return 0, false, fmt.Errorf("failed to update score: %w", err)
			}
		} else {
			// Update username in case it changed
			_, _ = db.Exec(`UPDATE level_leaderboards SET username = ? WHERE level = ? AND user_id = ?`,
				entry.Username, entry.Level, entry.UserID)
		}
	}

	// Calculate player's rank for this level (ordered by score DESC, then time_taken ASC)
	var rank int
	err = db.QueryRow(`
		SELECT COUNT(*) + 1 
		FROM level_leaderboards 
		WHERE level = ? AND (score > ? OR (score = ? AND time_taken < ?))`,
		entry.Level, entry.Score, entry.Score, entry.TimeTaken).Scan(&rank)
	if err != nil {
		rank = 1
	}

	return rank, isNew, nil
}

func parseLevels(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var list []string
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			var res []string
			for _, s := range list {
				if tr := strings.TrimSpace(s); tr != "" {
					res = append(res, tr)
				}
			}
			if res == nil {
				return []string{}
			}
			return res
		}
	}
	parts := strings.Split(raw, ",")
	var res []string
	for _, p := range parts {
		if tr := strings.TrimSpace(p); tr != "" {
			res = append(res, tr)
		}
	}
	if res == nil {
		return []string{}
	}
	return res
}

func serializeLevels(levels []string) string {
	if len(levels) == 0 {
		return "[]"
	}
	var clean []string
	for _, l := range levels {
		if tr := strings.TrimSpace(l); tr != "" {
			clean = append(clean, tr)
		}
	}
	if len(clean) == 0 {
		return "[]"
	}
	bytes, err := json.Marshal(clean)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

// CreateLeague inserts a new league into the leagues table with optional playable levels
func CreateLeague(name, description string, levels ...[]string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("league name cannot be empty")
	}
	lvlStr := "[]"
	if len(levels) > 0 && len(levels[0]) > 0 {
		lvlStr = serializeLevels(levels[0])
	}
	_, err := db.Exec(`INSERT INTO leagues (name, description, levels) VALUES (?, ?, ?)`, name, strings.TrimSpace(description), lvlStr)
	if err != nil {
		return fmt.Errorf("failed to create league (may already exist): %w", err)
	}
	return nil
}

// SetLeagueLevels updates the allowed playable levels for a league
func SetLeagueLevels(name string, levels []string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("league name cannot be empty")
	}
	lvlStr := serializeLevels(levels)
	res, err := db.Exec(`UPDATE leagues SET levels = ? WHERE name = ?`, lvlStr, name)
	if err != nil {
		return fmt.Errorf("failed to update league levels: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("league %q not found", name)
	}
	return nil
}

// GetLeagueLevels retrieves the allowed playable levels for a league
func GetLeagueLevels(name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return []string{}, nil
	}
	var raw string
	err := db.QueryRow(`SELECT COALESCE(levels, '') FROM leagues WHERE name = ?`, name).Scan(&raw)
	if err == sql.ErrNoRows {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parseLevels(raw), nil
}

// DeleteLeague removes a league from the leagues table
func DeleteLeague(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("league name cannot be empty")
	}
	res, err := db.Exec(`DELETE FROM leagues WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("failed to delete league: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("league %q not found", name)
	}
	return nil
}

// SetLeagueLock toggles the is_locked state of a league
func SetLeagueLock(name string, locked bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("league name cannot be empty")
	}
	lockVal := 0
	if locked {
		lockVal = 1
	}
	res, err := db.Exec(`UPDATE leagues SET is_locked = ? WHERE name = ?`, lockVal, name)
	if err != nil {
		return fmt.Errorf("failed to update league lock status: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("league %q not found", name)
	}
	return nil
}

// IsLeagueLocked checks if a league is currently locked
func IsLeagueLocked(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}
	var locked int
	err := db.QueryRow(`SELECT COALESCE(is_locked, 0) FROM leagues WHERE name = ?`, name).Scan(&locked)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return locked == 1, nil
}

// GetLeagueNames returns just a simple list of league names.
// Defaults to returning only unlocked leagues. If includeLocked is true, all leagues are returned.
func GetLeagueNames(includeLocked ...bool) ([]string, error) {
	query := `SELECT name FROM leagues WHERE COALESCE(is_locked, 0) = 0 ORDER BY name ASC`
	if len(includeLocked) > 0 && includeLocked[0] {
		query = `SELECT name FROM leagues ORDER BY name ASC`
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query league names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if names == nil {
		names = []string{}
	}
	return names, nil
}

// GetActiveLeagues returns list of leagues with their allowed playable levels
func GetActiveLeagues(includeLocked ...bool) ([]LeagueItem, error) {
	whereClause := "WHERE COALESCE(is_locked, 0) = 0"
	if len(includeLocked) > 0 && includeLocked[0] {
		whereClause = ""
	}
	query := fmt.Sprintf(`SELECT name, COALESCE(description, ''), COALESCE(levels, ''), COALESCE(is_locked, 0) FROM leagues %s ORDER BY name ASC`, whereClause)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active leagues: %w", err)
	}
	defer rows.Close()

	var items []LeagueItem
	for rows.Next() {
		var name, desc, rawLevels string
		var isLockedInt int
		if err := rows.Scan(&name, &desc, &rawLevels, &isLockedInt); err != nil {
			return nil, err
		}
		items = append(items, LeagueItem{
			Name:        name,
			Description: desc,
			Levels:      parseLevels(rawLevels),
			IsLocked:    isLockedInt != 0,
		})
	}
	if items == nil {
		items = []LeagueItem{}
	}
	return items, nil
}

// GetLeagues returns list of available leagues with summary statistics.
// By default returns only unlocked leagues unless includeLocked is true.
func GetLeagues(includeLocked ...bool) ([]LeagueInfo, error) {
	whereClause := "WHERE COALESCE(l.is_locked, 0) = 0"
	if len(includeLocked) > 0 && includeLocked[0] {
		whereClause = ""
	}
	query := fmt.Sprintf(`
		SELECT 
			l.name, 
			COALESCE(l.description, ''),
			COALESCE(l.levels, ''),
			COALESCE(l.is_locked, 0),
			COUNT(DISTINCT lb.user_id) as total_players,
			COUNT(DISTINCT lb.level) as total_levels,
			0 as top_score
		FROM leagues l
		LEFT JOIN level_leaderboards lb ON lb.league = l.name
		%s
		GROUP BY l.id, l.name, l.description, l.levels, l.is_locked
		ORDER BY total_players DESC, l.name ASC`, whereClause)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query leagues: %w", err)
	}
	defer rows.Close()

	var leagues []LeagueInfo
	for rows.Next() {
		var info LeagueInfo
		var rawLevels string
		var isLockedInt int
		if err := rows.Scan(&info.Name, &info.Description, &rawLevels, &isLockedInt, &info.TotalPlayers, &info.TotalLevels, &info.TopScore); err != nil {
			return nil, err
		}
		info.Levels = parseLevels(rawLevels)
		info.IsLocked = (isLockedInt != 0)

		// Find top player and their total weighted league score for this league
		_ = db.QueryRow(`
			SELECT username, ROUND(SUM(score * CAST(difficulty AS REAL)), 0)
			FROM level_leaderboards 
			WHERE league = ? 
			GROUP BY user_id, username
			ORDER BY SUM(score * CAST(difficulty AS REAL)) DESC, SUM(time_taken) ASC 
			LIMIT 1`, info.Name).Scan(&info.TopPlayer, &info.TopScore)

		leagues = append(leagues, info)
	}
	if leagues == nil {
		leagues = []LeagueInfo{}
	}
	return leagues, nil
}

// GetLeagueLeaderboard aggregates all level scores for each unique user_id within a league,
// scaling each level's base score by its difficulty multiplier:
// Total League Score = SUM(ROUND(score * CAST(difficulty AS REAL)))
func GetLeagueLeaderboard(league string) ([]LeagueEntry, error) {
	rows, err := db.Query(`
		SELECT 
			user_id,
			MAX(username) as username,
			ROUND(SUM(score * CAST(difficulty AS REAL)), 0) as total_score,
			COUNT(DISTINCT level) as levels_cleared,
			ROUND(SUM(time_taken), 2) as total_time,
			SUM(clicks) as total_clicks,
			MAX(submitted_at) as last_active
		FROM level_leaderboards
		WHERE league = ?
		GROUP BY user_id
		ORDER BY total_score DESC, total_time ASC, total_clicks ASC`, league)
	if err != nil {
		return nil, fmt.Errorf("failed to query league leaderboard: %w", err)
	}
	defer rows.Close()

	var entries []LeagueEntry
	rank := 1
	for rows.Next() {
		var e LeagueEntry
		e.League = league
		if err := rows.Scan(&e.UserID, &e.Username, &e.TotalScore, &e.LevelsCleared, &e.TotalTime, &e.TotalClicks, &e.LastActive); err != nil {
			return nil, err
		}
		e.Rank = rank
		rank++
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []LeagueEntry{}
	}
	return entries, nil
}

// GetLevels returns the list of all distinct levels with leaderboard data
func GetLevels() ([]LevelInfo, error) {
	rows, err := db.Query(`
		SELECT level, COUNT(*), MIN(time_taken), COALESCE(MAX(score), 0)
		FROM level_leaderboards
		GROUP BY level
		ORDER BY level ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query levels: %w", err)
	}
	defer rows.Close()

	var levels []LevelInfo
	for rows.Next() {
		var info LevelInfo
		if err := rows.Scan(&info.Name, &info.TotalScores, &info.BestTime, &info.TopScore); err != nil {
			return nil, err
		}

		_ = db.QueryRow(`
			SELECT username 
			FROM level_leaderboards 
			WHERE level = ? 
			ORDER BY score DESC, time_taken ASC 
			LIMIT 1`, info.Name).Scan(&info.TopPlayer)

		levels = append(levels, info)
	}
	if levels == nil {
		levels = []LevelInfo{}
	}
	return levels, nil
}

// GetLeaderboard returns ranked entries for a given level (ordered by score DESC, then time_taken ASC)
func GetLeaderboard(level string) ([]ScoreEntry, error) {
	rows, err := db.Query(`
		SELECT id, level, COALESCE(league, ''), user_id, username, time_taken, clicks, COALESCE(score, 0), COALESCE(difficulty, 'easy'), status, checkpoints, submitted_at
		FROM level_leaderboards
		WHERE level = ?
		ORDER BY score DESC, time_taken ASC, clicks ASC`, level)
	if err != nil {
		return nil, fmt.Errorf("failed to query leaderboard for level %s: %w", level, err)
	}
	defer rows.Close()

	var entries []ScoreEntry
	rank := 1
	for rows.Next() {
		var e ScoreEntry
		if err := rows.Scan(&e.ID, &e.Level, &e.League, &e.UserID, &e.Username, &e.TimeTaken, &e.Clicks, &e.Score, &e.Difficulty, &e.Status, &e.Checkpoints, &e.SubmittedAt); err != nil {
			return nil, err
		}
		e.Rank = rank
		rank++
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []ScoreEntry{}
	}
	return entries, nil
}
