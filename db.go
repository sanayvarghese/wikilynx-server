package main

import (
	"database/sql"
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
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	TotalPlayers int     `json:"totalPlayers"`
	TotalLevels  int     `json:"totalLevels"`
	TopPlayer    string  `json:"topPlayer"`
	TopScore     float64 `json:"topScore"`
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
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_leaderboards_level ON level_leaderboards(level);
	CREATE INDEX IF NOT EXISTS idx_leaderboards_rank ON level_leaderboards(level, time_taken ASC, clicks ASC);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create base schema: %w", err)
	}

	// Migrate missing columns if level_leaderboards already existed
	migrateColumns()

	// Seed default leagues
	seedLeagues()

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
		_, _ = db.Exec("ALTER TABLE level_leaderboards ADD COLUMN league TEXT DEFAULT 'Global Championship'")
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
}

func seedLeagues() {
	defaultLeagues := []struct {
		name string
		desc string
	}{
		{"Global Championship", "Official competitive league spanning all wiki speedrun levels."},
		{"Speedrun Masters", "High-intensity league emphasizing rapid traversal and link efficiency."},
		{"Wiki Explorers", "Casual league welcoming runs across all difficulty tiers."},
	}

	for _, l := range defaultLeagues {
		_, _ = db.Exec(`
			INSERT OR IGNORE INTO leagues (name, description)
			VALUES (?, ?)`, l.name, l.desc)
	}
}

func recalculateAllScores() {
	rows, err := db.Query(`
		SELECT id, time_taken, clicks, checkpoints 
		FROM level_leaderboards`)
	if err != nil {
		return
	}
	defer rows.Close()

	type item struct {
		id    int64
		score float64
	}
	var items []item

	for rows.Next() {
		var id int64
		var timeTaken float64
		var clicks int
		var chk int
		if err := rows.Scan(&id, &timeTaken, &clicks, &chk); err == nil {
			score := calculateBase(timeTaken, clicks, chk)
			items = append(items, item{id: id, score: score})
		}
	}

	for _, it := range items {
		_, _ = db.Exec("UPDATE level_leaderboards SET score = ? WHERE id = ?", it.score, it.id)
	}
}

func calculateBase(timeTaken float64, clicks int, checkpoints int) float64 {
	base := 10000.0 - (10.0 * timeTaken) - (100.0 * float64(clicks))
	if base < 100.0 {
		base = 100.0
	}
	base += float64(checkpoints) * 250.0
	return math.Round(base)
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

// CalculateScore computes the run's base score considering time, clicks, and checkpoints.
// For individual levels, all players compete on the pure unscaled base score:
//   Level Score = round( max(100, 10,000 - 10*Time - 100*Clicks) + (Checkpoints * 250) )
//
// The difficulty multiplier (clamped to 0.0 - 1.0) is stored alongside the run and is applied
// when combining multiple levels into a League Leaderboard:
//   League Level Contribution = round( Level Score * DifficultyMultiplier )
//
// Whatever the status may be (1 for win, 0 for lose), the exact same formula applies.
func CalculateScore(timeTaken float64, clicks int, diffVal interface{}, status int, checkpoints int) (float64, string) {
	_, diffName := ParseDifficultyMultiplier(diffVal)
	score := calculateBase(timeTaken, clicks, checkpoints)
	return score, diffName
}

// SaveScore records or updates a player's score for a level and league.
func SaveScore(entry ScoreEntry) (int, bool, error) {
	if entry.League == "" {
		entry.League = "Global Championship"
	}
	if entry.Difficulty == "" {
		entry.Difficulty = "easy"
	}

	// Ensure league exists in leagues table
	_, _ = db.Exec(`INSERT OR IGNORE INTO leagues (name, description) VALUES (?, 'Custom user-created league')`, entry.League)

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

// CreateLeague inserts a new league into the leagues table
func CreateLeague(name, description string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("league name cannot be empty")
	}
	_, err := db.Exec(`INSERT INTO leagues (name, description) VALUES (?, ?)`, name, strings.TrimSpace(description))
	if err != nil {
		return fmt.Errorf("failed to create league (may already exist): %w", err)
	}
	return nil
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

// GetLeagueNames returns just a simple list of league names
func GetLeagueNames() ([]string, error) {
	rows, err := db.Query(`SELECT name FROM leagues ORDER BY name ASC`)
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

// GetLeagues returns list of available leagues with summary statistics
func GetLeagues() ([]LeagueInfo, error) {
	rows, err := db.Query(`
		SELECT 
			l.name, 
			COALESCE(l.description, ''),
			COUNT(DISTINCT lb.user_id) as total_players,
			COUNT(DISTINCT lb.level) as total_levels,
			0 as top_score
		FROM leagues l
		LEFT JOIN level_leaderboards lb ON lb.league = l.name
		GROUP BY l.id, l.name, l.description
		ORDER BY total_players DESC, l.name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query leagues: %w", err)
	}
	defer rows.Close()

	var leagues []LeagueInfo
	for rows.Next() {
		var info LeagueInfo
		if err := rows.Scan(&info.Name, &info.Description, &info.TotalPlayers, &info.TotalLevels, &info.TopScore); err != nil {
			return nil, err
		}

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
		SELECT id, level, COALESCE(league, 'Global Championship'), user_id, username, time_taken, clicks, COALESCE(score, 0), COALESCE(difficulty, 'easy'), status, checkpoints, submitted_at
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
