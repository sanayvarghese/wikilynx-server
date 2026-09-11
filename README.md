# WikiLYNX Backend API Documentation

A standalone, high-performance Go server providing RESTful APIs for WikiLYNX speedrun & navigation leaderboards, competitive leagues, and composite scoring.

---

## Table of Contents

- [1. Overview & Base URL](#1-overview--base-url)
- [2. Core Concepts & Scoring Rules](#2-core-concepts--scoring-rules)
  - [Difficulty Multiplier (Always 0.0 – 1.0)](#difficulty-multiplier-always-00--10)
  - [Game Status (1 for Win, 0 for Lose)](#game-status-1-for-win-0-for-lose)
  - [Composite Score Calculation](#composite-score-calculation)
  - [League Aggregation Rules](#league-aggregation-rules)
  - [CORS Support](#cors-support)
- [3. Public Endpoints Reference](#3-public-endpoints-reference)
  - [3.1 `GET /api/leagues` (List Available League Names)](#31-get-apileagues)
  - [3.2 `POST /api/score` (Submit Level Completion Stats)](#32-post-apiscore)
  - [3.3 `GET /api/leaderboard` (Get Level or League Standings)](#33-get-apileaderboard)
  - [3.4 `GET /api/levels` (List Registered Levels)](#34-get-apilevels)
  - [3.5 `GET /api/health` (Server Health Check)](#35-get-apihealth)
- [4. Protected Admin Endpoints (`root` : `1234561`)](#4-protected-admin-endpoints)
  - [4.1 `POST /api/admin/leagues` (Create League)](#41-post-apiadminleagues)
  - [4.2 `DELETE /api/admin/leagues` (Delete League)](#42-delete-apiadminleagues)
  - [4.3 `POST /api/admin/leagues/lock` (Lock / Unlock League)](#43-post-apiadminleagueslock)
  - [4.4 `POST /api/admin/leagues/levels` (Configure Playable Levels)](#44-post-apiadminleagueslevels)
  - [4.5 `POST /api/admin/login` (Verify Admin Credentials)](#45-post-apiadminlogin)
- [5. Web Interfaces](#5-web-interfaces)
  - [Public Leaderboard: `http://localhost:8080/`](#public-leaderboard-httplocalhost8080)
  - [Admin Panel: `http://localhost:8080/admin`](#admin-panel-httplocalhost8080admin)
- [6. Qt / C++ Client Integration Example](#6-qt--c-client-integration-example)
  - [Header File (`wikilynx_api.h`)](#header-file-wikilynx_apih)
  - [Implementation File (`wikilynx_api.cpp`)](#implementation-file-wikilynx_apicpp)
- [7. How to Run the Server](#7-how-to-run-the-server)

---

## 1. Overview & Base URL

- **Default Base URL**: `http://localhost:8080`
- **Request / Response Format**: `application/json`
- **Database**: Embedded SQLite (`modernc.org/sqlite`, 100% pure Go, zero CGO/GCC dependencies)

---

## 2. Core Concepts & Scoring Rules

### Difficulty Multiplier (Always 0.0 – 1.0)

The level difficulty multiplier ($M_{\text{diff}}$) is **always strictly between 0.0 and 1.0**.

Pre-set difficulty names map to:
- `"easy"`: **`0.25`**
- `"medium"` / `"normal"`: **`0.50`**
- `"hard"`: **`0.75`**
- `"expert"` / `"insane"`: **`1.00`**
- Custom float: Any numeric value in range `[0.0, 1.0]` (values outside this range are automatically clamped to 0.0 – 1.0).

---

### Game Status (`1` for Win, `0` for Lose)

- **`1` = Win / Goal Reached**: Player successfully reached the target destination within the level's constraints.
- **`0` = Lose / Failed**: Player aborted, ran out of time, exceeded allowed clicks, or failed the challenge.

---

### Composite Score Calculation

Whatever the status may be (`1` for win or `0` for lose), the exact same base formula is applied:

$$\text{Completion Bonus} = \left(\frac{\text{Progress } \%}{100.0}\right) \times 2,500.0$$
$$\text{Base Score} = \max\left(100.0, 10,000.0 - (10.0 \times T) - (100.0 \times C)\right) + \text{Completion Bonus}$$

- **Single Level Leaderboards (`GET /api/leaderboard?level=...`)**:
  - Scores represent the pure **Base Score**: $\text{Level Score} = \text{round}(\text{Base Score})$.
  - Everyone playing the same level is judged on pure speed, route efficiency, and checkpoint completion percentage, keeping scores high, un-deflated, and normalized across levels with varying checkpoint counts.
- **Time ($T$) & Clicks ($C$)**: Lower time and fewer clicks maximize the base score.
- **Checkpoint Progress**: Full completion ($100\%$) awards the maximum $+2,500$ bonus points. Partial runs (losses/timeouts) receive proportional points (e.g. $50\% \rightarrow +1,250$ pts).
- **Status Independent**: The same formula applies for both wins (`1`) and losses/timeouts (`0`).

---

### League Combination & Difficulty Multiplier

The **difficulty multiplier ($0.0 \text{ to } 1.0$)** is stored with every run and is specifically applied when combining multiple levels into a **League Leaderboard**:

1. When players submit scores specifying a `league`, their runs are tagged with that league.
2. In the **League Leaderboard** (`GET /api/leaderboard?league=...`), scores from all distinct levels completed by a player are **weighted by level difficulty and summed by unique `userId`**:
   $$\text{League Level Contribution} = \text{round}(\text{Level Base Score} \times M_{\text{diff}})$$
   $$\text{Total League Score} = \sum_{\text{levels}} \left( \text{round}(\text{Level Base Score} \times M_{\text{diff}}) \right)$$
3. **Fair Cross-Level Balancing**: Harder levels (e.g. `insane = 1.00`) contribute full score, while easier levels (e.g. `easy = 0.25`) contribute proportionally, perfectly balancing players who choose different levels in the league.
4. Distinct players who share the same display name remain separate because aggregation keys on `userId`.

---

### CORS Support

All endpoints include CORS headers:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization, X-Admin-Password`

---

## 3. Public Endpoints Reference

### 3.1 `GET /api/leagues`

Returns active (unlocked) leagues along with their designated playable levels.

#### Request
- **Method**: `GET`
- **URL**: `/api/leagues`
- **Query Parameters**:
  - `detailed` *(optional, bool)*: If `"true"`, returns full objects with aggregate statistics (player counts, total levels tackled, top score).
  - `plain` or `namesOnly` *(optional, bool)*: If `"true"`, returns a simple string array of league names `["Global Championship", ...]`.

#### Success Response (Default - HTTP 200 OK)
```json
[
  {
    "name": "Global Championship",
    "description": "Official competitive league spanning all wiki speedrun levels.",
    "levels": []
  },
  {
    "name": "Speedrun Masters",
    "description": "High-intensity league emphasizing rapid traversal and link efficiency.",
    "levels": [
      "flyingduck",
      "cattomosfet!"
    ]
  },
  {
    "name": "Wiki Explorers",
    "description": "Casual league welcoming runs across all difficulty tiers.",
    "levels": []
  }
]
```

> [!NOTE]
> If `levels` is empty `[]`, **all levels** are permitted to be played in that league. If `levels` contains specific level names, only those designated levels are permitted.

#### Example curl:
```bash
curl http://localhost:8080/api/leagues
```

---

### 3.2 `POST /api/score`

Submits player stats and computes the level score.

> [!IMPORTANT]
> **Field Requirements**: Only `level`, `userId`, and `username` are **REQUIRED**. 
> Score calculation is driven by `primary`, `secondary`, and `tertiary` parameters. If omitted, they are automatically derived from legacy parameters (`time`, `clicks`, `checkpoints`, `progress`, `status`).

#### Request Headers
- `Content-Type: application/json`

#### Request Parameters:

| Field | Type | Required? | Allowed Values / Range | Description |
| :--- | :---: | :---: | :--- | :--- |
| `level` | `string` | **REQUIRED** | Non-empty string | Name of the level played (e.g. `"cattomosfet!"`) |
| `userId` | `string` | **REQUIRED** | Non-empty string | Unique persistent client ID (e.g. UUID) |
| `username` | `string` | **REQUIRED** | Non-empty string | Player's display name (e.g. `"SpeedDemon"`) |
| `primary` | `int` | *Optional* | $\ge 0$ | **Checkpoints completed** raw count (e.g. `4`) |
| `secondary` | `int`/`float` | *Optional* | $\ge 0$ | **Time in milliseconds** (e.g. `45000` ms) |
| `tertiary` | `float`/`int` | *Optional* | $\ge 0$ | Raw clicks (optional, multiplied by 0 in score) |
| `difficulty` | `float`/`string` | *Optional* | `0.0` – `1.0` or `"easy"`, `"medium"`, `"hard"`, `"insane"` | Difficulty multiplier (defaults to `0.25`) |
| `status` | `int` | *Optional* | **`1` = Win, `0` = Lose** | Result of the run |
| `league` | `string` | *Optional* | String | Target league (defaults to `""` for non-league run) |

#### Score Equation (Max Base Score = 1,100 pts):
$$\text{RankPct}(\text{checkpoint}) = \frac{\text{checkpoint}}{\max(1, \text{max\_checkpoint})}$$

$$\text{RankPct}(\text{time}) = \begin{cases} 
1.0 & \text{if } \text{max\_time} == \text{min\_time} \\ 
\max\left(0.0,\, \min\left(1.0,\, \frac{\text{max\_time} - \text{time\_ms}}{\text{max\_time} - \text{min\_time}}\right)\right) & \text{otherwise} 
\end{cases}$$

$$\mathbf{\text{Base Score}} = \begin{cases} 
0 & \text{if } \text{checkpoint} \le 0 \text{ (early quit = 0 pts)} \\ 
\text{round}(1000 \times \text{RankPct}(\text{checkpoint}) + 100 \times \text{RankPct}(\text{time}) + 0 \times \text{tertiary}) & \text{otherwise} 
\end{cases}$$

$$\mathbf{\text{Final League Score}} = \text{round}(\text{Base Score} \times \text{DifficultyMultiplier})$$

#### Example Request:
```json
{
  "level": "cattomosfet!",
  "userId": "c7a8109d-8d54-46e3-a442-870b2241cf89",
  "username": "SpeedDemon",
  "primary": 4,
  "secondary": 20000,
  "tertiary": 5,
  "difficulty": 0.75,
  "league": "College Champions"
}
```

#### Success Response (HTTP 200 OK):
```json
{
  "success": true,
  "message": "Score recorded successfully",
  "rank": 1,
  "score": 1100,
  "isNew": true,
  "data": {
    "id": 0,
    "rank": 0,
    "level": "cattomosfet!",
    "league": "College Champions",
    "userId": "c7a8109d-8d54-46e3-a442-870b2241cf89",
    "username": "SpeedDemon",
    "timeTaken": 20.0,
    "clicks": 5,
    "score": 1100,
    "difficulty": "0.75",
    "status": 1,
    "checkpoints": 4,
    "submittedAt": ""
  }
}
```

#### Error Responses (HTTP 400 Bad Request):
If any required field is omitted, the API responds with an explicit error:
```json
{ "success": false, "message": "level is required" }
```
```json
{ "success": false, "message": "userId is required" }
```
```json
{ "success": false, "message": "username is required" }
```
```json
{ "success": false, "message": "time is required" }
```
```json
{ "success": false, "message": "clicks is required" }
```
```json
{ "success": false, "message": "difficulty is required (0.0 to 1.0 or 'easy', 'medium', 'hard', 'insane')" }
```
```json
{ "success": false, "message": "status is required (1 for win, 0 for lose)" }
```
```json
{ "success": false, "message": "checkpoints is required (pass 0 if none)" }
```

#### Example curl:
```bash
curl -X POST http://localhost:8080/api/score \
  -H "Content-Type: application/json" \
  -d '{
    "level": "flyingduck",
    "userId": "usr-123",
    "username": "Alice",
    "time": 18.5,
    "clicks": 3,
    "difficulty": 0.25,
    "status": 1,
    "checkpoints": 2,
    "league": "Global Championship"
  }'
```

---

### 3.3 `GET /api/leaderboard`

Retrieves ranked standings. Supports both single level and aggregated league leaderboards.

#### Mode A: Single Level Leaderboard (`?level=<name>`)
- **URL**: `/api/leaderboard?level=flyingduck`
- **Sort Order**: `score DESC, time_taken ASC, clicks ASC`
- **Response Schema**:
```json
{
  "type": "level",
  "level": "flyingduck",
  "count": 2,
  "scores": [
    {
      "id": 1,
      "rank": 1,
      "level": "flyingduck",
      "league": "Speedrun Masters",
      "userId": "usr-winner",
      "username": "SpeedDemon",
      "timeTaken": 18.5,
      "clicks": 3,
      "score": 2429,
      "difficulty": "0.25",
      "status": 1,
      "checkpoints": 2,
      "submittedAt": "2026-09-10T14:35:52Z"
    },
    {
      "id": 2,
      "rank": 2,
      "level": "flyingduck",
      "league": "Speedrun Masters",
      "userId": "usr-loser",
      "username": "UnluckyPlayer",
      "timeTaken": 75.2,
      "clicks": 20,
      "score": 250,
      "difficulty": "0.25",
      "status": 0,
      "checkpoints": 2,
      "submittedAt": "2026-09-10T14:35:52Z"
    }
  ]
}
```

#### Mode B: Aggregated League Leaderboard (`?league=<name>`)
- **URL**: `/api/leaderboard?league=Speedrun%20Masters`
- **Sort Order**: `totalScore DESC, totalTime ASC, totalClicks ASC`
- **Response Schema**:
```json
{
  "type": "league",
  "league": "Speedrun Masters",
  "count": 2,
  "scores": [
    {
      "rank": 1,
      "league": "Speedrun Masters",
      "userId": "usr-winner",
      "username": "SpeedDemon",
      "totalScore": 10042,
      "levelsCleared": 2,
      "totalTime": 38.5,
      "totalClicks": 7,
      "lastActive": "2026-09-10 14:35:52"
    }
  ]
}
```

---

### 3.4 `GET /api/levels`

Retrieves a list of all distinct levels with registered scores.

#### Request
- **Method**: `GET`
- **URL**: `/api/levels`

#### Success Response (HTTP 200 OK)
```json
[
  {
    "name": "cattomosfet!",
    "totalScores": 3,
    "bestTime": 20.0,
    "topPlayer": "SpeedDemon",
    "topScore": 7613
  },
  {
    "name": "flyingduck",
    "totalScores": 5,
    "bestTime": 18.5,
    "topPlayer": "SpeedDemon",
    "topScore": 2429
  }
]
```

---

### 3.5 `GET /api/health`

Health check endpoint.

#### Request
- **Method**: `GET`
- **URL**: `/api/health`

#### Success Response (HTTP 200 OK)
```json
{
  "status": "healthy",
  "time": "2026-09-10T14:40:00Z"
}
```

---

## 4. Protected Admin Endpoints

Protected with credentials:
- **Admin ID**: `root`
- **Password**: `1234561`

Authentication can be passed via:
- HTTP Basic Auth header: `Authorization: Basic cm9vdDoxMjM0NTYx`
- Custom header: `X-Admin-Password: 1234561`

---

### 4.1 `POST /api/admin/leagues`

Creates a new league.

#### Request
- **Method**: `POST`
- **URL**: `/api/admin/leagues`
- **Headers**:
  - `Content-Type: application/json`
  - `Authorization: Basic cm9vdDoxMjM0NTYx`
- **Body**:
  ```json
  {
    "name": "Weekend Blitz",
    "description": "Short sprint challenges every weekend",
    "levels": ["flyingduck", "cattomosfet!"]
  }
  ```
  *(Note: `levels` is optional. Pass an array of level names or a comma-separated string, or omit/leave empty to permit all levels).*
- **Response**: `{"success": true, "message": "League created successfully"}`

#### Example curl:
```bash
curl -X POST http://localhost:8080/api/admin/leagues \
  -u root:1234561 \
  -H "Content-Type: application/json" \
  -d '{"name": "Weekend Blitz", "description": "Short sprint challenges", "levels": ["flyingduck", "cattomosfet!"]}'
```

---

### 4.2 `DELETE /api/admin/leagues`

Deletes an existing league.

#### Request
- **Method**: `DELETE`
- **URL**: `/api/admin/leagues?name=Weekend%20Blitz`
- **Headers**:
  - `Authorization: Basic cm9vdDoxMjM0NTYx`
- **Response**: `{"success": true, "message": "League deleted successfully"}`

#### Example curl:
```bash
curl -X DELETE "http://localhost:8080/api/admin/leagues?name=Weekend%20Blitz" \
  -u root:1234561
```

---

### 4.3 `POST /api/admin/leagues/lock`

Locks or unlocks an existing league. When a league is **locked**:
- It is hidden from `GET /api/leagues`, so game clients will not see or be able to select it.
- Any score submissions (`POST /api/score`) submitted under a locked league are rejected with `HTTP 403 Forbidden`.

#### Request
- **Method**: `POST` (or `PATCH` to `/api/admin/leagues`)
- **URL**: `/api/admin/leagues/lock`
- **Headers**:
  - `Content-Type: application/json`
  - `Authorization: Basic cm9vdDoxMjM0NTYx`
- **Body**:
  ```json
  {
    "name": "Speedrun Masters",
    "locked": true
  }
  ```
- **Response**: `{"success": true, "message": "League \"Speedrun Masters\" successfully locked"}`

#### Example curl (Lock):
```bash
curl -X POST http://localhost:8080/api/admin/leagues/lock \
  -u root:1234561 \
  -H "Content-Type: application/json" \
  -d '{"name": "Speedrun Masters", "locked": true}'
```

#### Example curl (Unlock):
```bash
curl -X POST http://localhost:8080/api/admin/leagues/lock \
  -u root:1234561 \
  -H "Content-Type: application/json" \
  -d '{"name": "Speedrun Masters", "locked": false}'
```

---

### 4.4 `POST /api/admin/leagues/levels`

Configures or updates the allowed playable levels for an existing league.

#### Request
- **Method**: `POST`, `PUT`, or `PATCH`
- **URL**: `/api/admin/leagues/levels`
- **Headers**:
  - `Content-Type: application/json`
  - `Authorization: Basic cm9vdDoxMjM0NTYx`
- **Body**:
  ```json
  {
    "name": "Speedrun Masters",
    "levels": ["flyingduck", "cattomosfet!"]
  }
  ```
  *(Pass an empty array `[]` or empty string `""` to remove restrictions and permit all levels).*
- **Response**:
  ```json
  {
    "success": true,
    "message": "Playable levels updated for league \"Speedrun Masters\"",
    "data": ["flyingduck", "cattomosfet!"]
  }
  ```

#### Example curl:
```bash
curl -X POST http://localhost:8080/api/admin/leagues/levels \
  -u root:1234561 \
  -H "Content-Type: application/json" \
  -d '{"name": "Speedrun Masters", "levels": ["flyingduck", "cattomosfet!"]}'
```

---

### 4.5 `POST /api/admin/login`

Verifies admin credentials for web forms.

- **Body**: `{"username": "root", "password": "1234561"}`
- **Response**: `{"success": true, "message": "Authentication successful"}`

---

## 5. Web Interfaces

### Public Leaderboard: `http://localhost:8080/`
- **Dropdown**: Organized into `🏆 Competitive Leagues` and `🎮 Individual Levels` (displays only active, unlocked leagues).
- **Ranked Table**: Displays Rank (🥇, 🥈, 🥉), Player & `userId`, Score Badge (`★ 7,613 pts`), Time, Clicks, Difficulty (`0.0 - 1.0`), Status (`Win (1)` / `Loss (0)`), Date.
- **Highlights Cards**: Top Score, Total Competitors, Best Route / Levels Cleared, Active View.
- **Search & Auto-Refresh**: Instant live filter with optional 10s auto-refresh.

### Admin Panel: `http://localhost:8080/admin`
- **Security**: Password prompt requiring `root` / `1234561`.
- **League Management**: 
  - **Create League with Playable Levels**: Set optional comma-separated playable level names during creation.
  - **Edit Playable Levels**: Single-click `[ 🎯 Levels ]` button on any league row to modify or clear its permitted levels on the fly.
  - **Lock / Unlock Toggle**: Single-click `[ 🔒 Lock ]` / `[ 🔓 Unlock ]` button to hide/show leagues for clients and disable/enable score submissions.
  - **Badges**: Displays `🟢 Active` / `🔒 Locked` status, and blue badge chips for each designated playable level (or "All Levels Allowed").


---

## 7. How to Run the Server

### Option 1: Run Precompiled Executable
```bash
cd server
./wikilynx-server.exe --port 8080
```

### Option 2: Run with Go
```bash
cd server
go run . --port 8080
```
*(If `go` is not in your shell's PATH on Windows: `"/c/Program Files/Go/bin/go" run .`)*
