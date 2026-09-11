package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ScoreRequest struct {
	Level       string      `json:"level"`
	League      string      `json:"league"`
	Difficulty  interface{} `json:"difficulty,omitempty"` // "easy", "medium", "hard", or numeric float
	UserID      string      `json:"userId"`
	Username    string      `json:"username"`
	Time        interface{} `json:"time,omitempty"`
	Clicks      interface{} `json:"clicks,omitempty"`
	Status      interface{} `json:"status,omitempty"` // 0 = Lose, 1 = Win (accepts int, bool, or string)
	Checkpoints interface{} `json:"checkpoints,omitempty"`
	Progress    interface{} `json:"progress,omitempty"`

	// Score parameters:
	Primary   interface{} `json:"primary,omitempty"`   // Progress ratio (done/total checkpoints)
	Secondary interface{} `json:"secondary,omitempty"` // Time ratio ((totalTime - timeTaken) / totalTime)
	Tertiary  interface{} `json:"tertiary,omitempty"`  // Raw click count
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Rank    int         `json:"rank,omitempty"`
	Score   float64     `json:"score,omitempty"`
	IsNew   bool        `json:"isNew,omitempty"`
}

func parseStatus(val interface{}) int {
	if val == nil {
		return 1 // Default: Win (1)
	}
	switch v := val.(type) {
	case bool:
		if v {
			return 1
		}
		return 0
	case float64:
		if int(v) == 0 {
			return 0
		}
		return 1
	case int:
		if v == 0 {
			return 0
		}
		return 1
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		if s == "0" || s == "false" || s == "lose" || s == "loss" || s == "lost" || s == "failed" {
			return 0
		}
		return 1
	default:
		return 1
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func parseNumber(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	default:
		return 0, fmt.Errorf("unexpected numeric type: %T", val)
	}
}

func handleScoreSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req ScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "Invalid JSON: " + err.Error()})
		return
	}

	level := strings.TrimSpace(req.Level)
	if level == "" {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "level is required"})
		return
	}

	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "userId is required"})
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "username is required"})
		return
	}

	// Optional fields parsing
	var timeTaken float64 = 0.0
	if req.Time != nil {
		if t, err := parseNumber(req.Time); err == nil && t >= 0 {
			timeTaken = t
		}
	}

	var clicks int = 0
	if req.Clicks != nil {
		if c, err := parseNumber(req.Clicks); err == nil && c >= 0 {
			clicks = int(c)
		}
	}

	status := parseStatus(req.Status)

	var checkpoints int = 0
	if req.Checkpoints != nil {
		if chkF, err := parseNumber(req.Checkpoints); err == nil && chkF >= 0 {
			checkpoints = int(chkF)
		}
	}

	// Primary parameter: Progress ratio (0.0 to 1.0)
	var primary float64 = 0.0
	if req.Primary != nil {
		if p, err := parseNumber(req.Primary); err == nil {
			primary = p
		}
	} else if req.Progress != nil {
		if p, err := parseNumber(req.Progress); err == nil {
			primary = p / 100.0
		}
	} else if checkpoints > 0 {
		primary = float64(checkpoints) / 4.0
		if primary > 1.0 {
			primary = 1.0
		}
	} else if status == 1 {
		primary = 1.0
	}

	// Secondary parameter: Time ratio (0.0 to 1.0)
	var secondary float64 = 0.0
	if req.Secondary != nil {
		if s, err := parseNumber(req.Secondary); err == nil {
			secondary = s
		}
	} else if timeTaken > 0 {
		secondary = math.Max(0.0, (600.0-timeTaken)/600.0)
	} else if status == 1 {
		secondary = 1.0
	}

	// Tertiary parameter: Raw click count
	var tertiary float64 = 0.0
	if req.Tertiary != nil {
		if t, err := parseNumber(req.Tertiary); err == nil && t >= 0 {
			tertiary = t
		}
	} else {
		tertiary = float64(clicks)
	}

	if clicks == 0 && tertiary > 0 {
		clicks = int(tertiary)
	}

	league := strings.TrimSpace(req.League)
	if league != "" {
		locked, err := IsLeagueLocked(league)
		if err == nil && locked {
			log.Printf("[Score Rejected] Submission attempted to locked league %q by %s (%s)", league, username, userID)
			jsonResponse(w, http.StatusForbidden, APIResponse{
				Success: false,
				Message: fmt.Sprintf("League %q is locked and is not currently accepting score submissions", league),
			})
			return
		}

		allowedLevels, err := GetLeagueLevels(league)
		if err == nil && len(allowedLevels) > 0 {
			allowed := false
			for _, al := range allowedLevels {
				if strings.EqualFold(strings.TrimSpace(al), level) {
					allowed = true
					break
				}
			}
			if !allowed {
				log.Printf("[Score Rejected] Level %q is not permitted in league %q (allowed: %v) by %s (%s)",
					level, league, allowedLevels, username, userID)
				jsonResponse(w, http.StatusBadRequest, APIResponse{
					Success: false,
					Message: fmt.Sprintf("Level %q is not a permitted playable level in league %q. Allowed: %s", level, league, strings.Join(allowedLevels, ", ")),
				})
				return
			}
		}
	}

	// Compute score based on primary, secondary, tertiary, and difficulty
	score, diffName := CalculateScore(primary, secondary, tertiary, req.Difficulty)

	entry := ScoreEntry{
		Level:       level,
		League:      league,
		UserID:      userID,
		Username:    username,
		TimeTaken:   timeTaken,
		Clicks:      clicks,
		Score:       score,
		Difficulty:  diffName,
		Status:      status,
		Checkpoints: checkpoints,
	}

	rank, isNew, err := SaveScore(entry)
	if err != nil {
		log.Printf("Error saving score: %v", err)
		jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: "Database error: " + err.Error()})
		return
	}

	log.Printf("[Score Submitted] Level=%s, League=%s, Player=%s (%s), Time=%.2fs, Clicks=%d, Diff=%s, Score=%.0f, Rank=%d",
		level, league, username, userID, timeTaken, clicks, diffName, score, rank)

	jsonResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: "Score recorded successfully",
		Rank:    rank,
		Score:   score,
		IsNew:   isNew,
		Data:    entry,
	})
}

func handleGetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	leagueParam := strings.TrimSpace(r.URL.Query().Get("league"))
	levelParam := strings.TrimSpace(r.URL.Query().Get("level"))

	// If league query parameter is provided, return aggregated league leaderboard
	if leagueParam != "" {
		entries, err := GetLeagueLeaderboard(leagueParam)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"type":    "league",
			"league":  leagueParam,
			"count":   len(entries),
			"scores":  entries,
		})
		return
	}

	// Otherwise level leaderboard
	level := levelParam
	if level == "" {
		levels, err := GetLevels()
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
			return
		}
		if len(levels) > 0 {
			level = levels[0].Name
		} else {
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"type":   "level",
				"level":  "",
				"count":  0,
				"scores": []ScoreEntry{},
			})
			return
		}
	}

	scores, err := GetLeaderboard(level)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"type":   "level",
		"level":  level,
		"count":  len(scores),
		"scores": scores,
	})
}

const (
	AdminUser = "root"
	AdminPass = "1234561"
)

func checkAdminAuth(r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	if ok && u == AdminUser && p == AdminPass {
		return true
	}
	if r.Header.Get("X-Admin-Password") == AdminPass || r.Header.Get("X-Admin-Key") == AdminPass {
		return true
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") && strings.TrimPrefix(authHeader, "Bearer ") == AdminPass {
		return true
	}
	return false
}

func requireAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	if !checkAdminAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="WikiLYNX Admin"`)
		jsonResponse(w, http.StatusUnauthorized, APIResponse{
			Success: false,
			Message: "Unauthorized: Admin credentials (root:1234561) required",
		})
		return false
	}
	return true
}

func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
		return
	}
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&creds)
	if creds.Username == AdminUser && creds.Password == AdminPass {
		jsonResponse(w, http.StatusOK, APIResponse{
			Success: true,
			Message: "Authentication successful",
			Data:    map[string]string{"token": AdminPass},
		})
		return
	}
	jsonResponse(w, http.StatusUnauthorized, APIResponse{Success: false, Message: "Invalid admin credentials"})
}

func handleAdminLeagueLock(w http.ResponseWriter, r *http.Request) {
	if !requireAdminAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
		return
	}
	var req struct {
		Name   string `json:"name"`
		Locked *bool  `json:"locked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "Invalid JSON: " + err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "League name is required"})
		return
	}
	if req.Locked == nil {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "'locked' (boolean) is required"})
		return
	}
	if err := SetLeagueLock(name, *req.Locked); err != nil {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: err.Error()})
		return
	}
	action := "unlocked"
	if *req.Locked {
		action = "locked"
	}
	log.Printf("[Admin] League %q has been %s", name, action)
	jsonResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("League %q successfully %s", name, action),
	})
}

func handleAdminLeagues(w http.ResponseWriter, r *http.Request) {
	if !requireAdminAuth(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Admin gets all leagues including locked ones
		leagues, err := GetLeagues(true)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, leagues)

	case http.MethodPost:
		var req struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Levels      interface{} `json:"levels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "Invalid JSON: " + err.Error()})
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "League name is required"})
			return
		}
		levels := parseLevelsInput(req.Levels)
		if err := CreateLeague(name, req.Description, levels); err != nil {
			jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: err.Error()})
			return
		}
		log.Printf("[Admin] Created League: %s (playable levels: %v)", name, levels)
		jsonResponse(w, http.StatusOK, APIResponse{Success: true, Message: "League created successfully"})

	case http.MethodPatch:
		handleAdminLeagueLock(w, r)

	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			var req struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			name = strings.TrimSpace(req.Name)
		}
		if name == "" {
			jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "League name is required to delete"})
			return
		}
		if err := DeleteLeague(name); err != nil {
			jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: err.Error()})
			return
		}
		log.Printf("[Admin] Deleted League: %s", name)
		jsonResponse(w, http.StatusOK, APIResponse{Success: true, Message: "League deleted successfully"})

	default:
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
	}
}

func parseLevelsInput(val interface{}) []string {
	if val == nil {
		return []string{}
	}
	var levels []string
	switch v := val.(type) {
	case []interface{}:
		for _, item := range v {
			if s := strings.TrimSpace(fmt.Sprintf("%v", item)); s != "" {
				levels = append(levels, s)
			}
		}
	case []string:
		for _, s := range v {
			if tr := strings.TrimSpace(s); tr != "" {
				levels = append(levels, tr)
			}
		}
	case string:
		parts := strings.Split(v, ",")
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				levels = append(levels, s)
			}
		}
	}
	if levels == nil {
		return []string{}
	}
	return levels
}

func handleAdminLeagueLevels(w http.ResponseWriter, r *http.Request) {
	if !requireAdminAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPatch && r.Method != http.MethodPut {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
		return
	}
	var req struct {
		Name   string      `json:"name"`
		Levels interface{} `json:"levels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "Invalid JSON: " + err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: "League name is required"})
		return
	}
	levels := parseLevelsInput(req.Levels)
	if err := SetLeagueLevels(name, levels); err != nil {
		jsonResponse(w, http.StatusBadRequest, APIResponse{Success: false, Message: err.Error()})
		return
	}
	log.Printf("[Admin] League %q playable levels updated: %v", name, levels)
	jsonResponse(w, http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Playable levels updated for league %q", name),
		Data:    levels,
	})
}

func handleGetLeagues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	includeLocked := r.URL.Query().Get("includeLocked") == "true"

	// If detailed=true, return full objects with player counts and stats
	if r.URL.Query().Get("detailed") == "true" {
		leagues, err := GetLeagues(includeLocked)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, leagues)
		return
	}

	// If plain string list is requested (for backwards compatibility)
	if r.URL.Query().Get("plain") == "true" || r.URL.Query().Get("namesOnly") == "true" {
		names, err := GetLeagueNames(includeLocked)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, names)
		return
	}

	// Default: return list of active leagues with allowed playable levels
	items, err := GetActiveLeagues(includeLocked)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, items)
}

func handleGetLevels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	levels, err := GetLevels()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, APIResponse{Success: false, Message: err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, levels)
}

func main() {
	portFlag := flag.String("port", "8080", "HTTP server port")
	dbFlag := flag.String("db", "wikilynx.db", "SQLite database path")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = *portFlag
	}

	// Initialize DB
	database, err := initDB(*dbFlag)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()

	// Public API Endpoints
	mux.HandleFunc("/api/score", handleScoreSubmit)
	mux.HandleFunc("/api/leaderboard", handleGetLeaderboard)
	mux.HandleFunc("/api/leagues", handleGetLeagues)
	mux.HandleFunc("/api/levels", handleGetLevels)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status": "healthy",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Admin API Endpoints (Protected with root:1234561)
	mux.HandleFunc("/api/admin/login", handleAdminLogin)
	mux.HandleFunc("/api/admin/leagues", handleAdminLeagues)
	mux.HandleFunc("/api/admin/leagues/lock", handleAdminLeagueLock)
	mux.HandleFunc("/api/admin/leagues/levels", handleAdminLeagueLevels)

	// Admin Web Interface Page
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(".", "web", "admin.html"))
	})

	// Static files for public web interface
	webDir := filepath.Join(".", "web")
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		_ = os.MkdirAll(webDir, 0755)
	}
	fileServer := http.FileServer(http.Dir(webDir))
	mux.Handle("/", fileServer)

	addr := ":" + port
	fmt.Println("=========================================")
	fmt.Println("   WikiLYNX Level & League Leaderboards")
	fmt.Printf("   Listening on http://localhost:%s\n", port)
	fmt.Println("   Endpoints:")
	fmt.Println("     GET    /                          (Public Web Interface)")
	fmt.Println("     GET    /admin                     (Secured Admin Web Interface)")
	fmt.Println("     POST   /api/score                 (Submit Score)")
	fmt.Println("     GET    /api/leaderboard           (Get Scores)")
	fmt.Println("     GET    /api/leagues               (List of Active Leagues & Levels)")
	fmt.Println("     POST   /api/admin/leagues         (Create League - Auth Required)")
	fmt.Println("     POST   /api/admin/leagues/lock    (Lock/Unlock League - Auth Required)")
	fmt.Println("     POST   /api/admin/leagues/levels  (Set Playable Levels - Auth Required)")
	fmt.Println("     DELETE /api/admin/leagues         (Delete League - Auth Required)")
	fmt.Println("=========================================")

	handler := corsMiddleware(mux)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
