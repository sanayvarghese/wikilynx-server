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
  - [4.3 `POST /api/admin/login` (Verify Admin Credentials)](#43-post-apiadminlogin)
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

All parameters (`time`, `clicks`, `difficulty`, `status`, `checkpoints`) directly factor into the final score:

#### When Player Wins (`status == 1`):
$$\text{Base} = \max\left(100.0, 10,000.0 - (10.0 \times T) - (100.0 \times C)\right) + (\text{checkpoints} \times 250.0)$$
$$\text{Score} = \text{round}(M_{\text{diff}} \times \text{Base})$$

- High speed ($T$) and low clicks ($C$) maximize the base points.
- Each cleared checkpoint awards $+250$ bonus points.
- Scaled by the difficulty multiplier ($0.0 \text{ to } 1.0$).

#### When Player Loses (`status == 0`):
$$\text{Score} = \text{round}(M_{\text{diff}} \times (\text{checkpoints} \times 250.0))$$

- Rewards progress: if the player cleared 2 checkpoints before failing on a medium level ($0.50$), they still earn $2 \times 250 \times 0.50 = 250$ points!
- If no checkpoints were cleared (`checkpoints == 0`), the loss score is $0$.

---

### League Aggregation Rules

1. When players submit scores specifying a `league`, their runs are tagged with that league.
2. In the **League Leaderboard** (`GET /api/leaderboard?league=...`), scores from all distinct levels completed by a player are **combined by unique `userId`**:
   - Distinct players who share the same display name remain separate because aggregation keys on `userId`.
   - Harder levels yield more points, so players playing different levels are compared fairly.
   - $\text{Total League Score} = \sum \text{Best Score per Level}$.

---

### CORS Support

All endpoints include CORS headers:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization, X-Admin-Password`

---

## 3. Public Endpoints Reference

### 3.1 `GET /api/leagues`

Returns a list of all currently available leagues.

#### Request
- **Method**: `GET`
- **URL**: `/api/leagues`
- **Query Parameters**:
  - `detailed` *(optional, bool)*: If `"true"`, returns an array of objects with player counts and top scores. Default returns a clean JSON array of strings.

#### Success Response (Default - HTTP 200 OK)
```json
[
  "Global Championship",
  "Speedrun Masters",
  "Wiki Explorers"
]
```

#### Example curl:
```bash
curl http://localhost:8080/api/leagues
```

---

### 3.2 `POST /api/score`

Submits player stats upon finishing a level.

> [!IMPORTANT]
> **Field Requirements**: Except for `league` (which is optional and defaults to `"Global Championship"`), **ALL other fields are REQUIRED** because they are all used in the score calculation. If the player cleared no checkpoints, pass `0`.

#### Request Headers
- `Content-Type: application/json`

#### Request Parameters:

| Field | Type | Required? | Allowed Values / Range | Description |
| :--- | :---: | :---: | :--- | :--- |
| `level` | `string` | **REQUIRED** | Non-empty string | Name of the level played (e.g. `"flyingduck"`) |
| `userId` | `string` | **REQUIRED** | Non-empty string | Unique persistent client ID (e.g. UUID) |
| `username` | `string` | **REQUIRED** | Non-empty string | Player's display name (e.g. `"Alice"`) |
| `time` | `float` | **REQUIRED** | $\ge 0.0$ | Total time taken in seconds (e.g. `24.5`) |
| `clicks` | `int` | **REQUIRED** | $\ge 0$ | Total links clicked (e.g. `5`) |
| `difficulty` | `float`/`string` | **REQUIRED** | `0.0` – `1.0` or `"easy"`, `"medium"`, `"hard"`, `"insane"` | Multiplier between 0.0 and 1.0 |
| `status` | `int` | **REQUIRED** | **`1` = Win, `0` = Lose** | Result of the run |
| `checkpoints` | `int` | **REQUIRED** | $\ge 0$ (pass `0` if none) | Checkpoints cleared |
| `league` | `string` | *Optional* | String | Target league (default: `"Global Championship"`) |

#### Example Request (Win):
```json
{
  "level": "cattomosfet!",
  "userId": "c7a8109d-8d54-46e3-a442-870b2241cf89",
  "username": "SpeedDemon",
  "time": 20.0,
  "clicks": 4,
  "difficulty": 0.75,
  "status": 1,
  "checkpoints": 3,
  "league": "Speedrun Masters"
}
```

#### Example Request (Loss with partial checkpoints):
```json
{
  "level": "cattomosfet!",
  "userId": "u-failed-runner",
  "username": "UnluckyPlayer",
  "time": 60.0,
  "clicks": 15,
  "difficulty": 0.50,
  "status": 0,
  "checkpoints": 2,
  "league": "Speedrun Masters"
}
```

#### Success Response (HTTP 200 OK):
```json
{
  "success": true,
  "message": "Score recorded successfully",
  "rank": 1,
  "score": 7613,
  "isNew": true,
  "data": {
    "id": 0,
    "rank": 0,
    "level": "cattomosfet!",
    "league": "Speedrun Masters",
    "userId": "c7a8109d-8d54-46e3-a442-870b2241cf89",
    "username": "SpeedDemon",
    "timeTaken": 20.0,
    "clicks": 4,
    "score": 7613,
    "difficulty": "0.75",
    "status": 1,
    "checkpoints": 3,
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
    "description": "Short sprint challenges every weekend"
  }
  ```
- **Response**: `{"success": true, "message": "League created successfully"}`

#### Example curl:
```bash
curl -X POST http://localhost:8080/api/admin/leagues \
  -u root:1234561 \
  -H "Content-Type: application/json" \
  -d '{"name": "Weekend Blitz", "description": "Short sprint challenges"}'
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

### 4.3 `POST /api/admin/login`

Verifies admin credentials for web forms.

- **Body**: `{"username": "root", "password": "1234561"}`
- **Response**: `{"success": true, "message": "Authentication successful"}`

---

## 5. Web Interfaces

### Public Leaderboard: `http://localhost:8080/`
- **Dropdown**: Organized into `🏆 Competitive Leagues` and `🎮 Individual Levels`.
- **Ranked Table**: Displays Rank (🥇, 🥈, 🥉), Player & `userId`, Score Badge (`★ 7,613 pts`), Time, Clicks, Difficulty (`0.0 - 1.0`), Status (`Win (1)` / `Loss (0)`), Date.
- **Highlights Cards**: Top Score, Total Competitors, Best Route / Levels Cleared, Active View.
- **Search & Auto-Refresh**: Instant live filter with optional 10s auto-refresh.

### Admin Panel: `http://localhost:8080/admin`
- **Security**: Password prompt requiring `root` / `1234561`.
- **League Management**: Create new leagues (Name & Description) or delete existing leagues with one click.

---

## 6. Complete Qt / C++ Client Integration Example

Drop this client helper into your Qt C++ client application to communicate with the Go server.

### Header File (`wikilynx_api.h`)

```cpp
#ifndef WIKILYNX_API_H
#define WIKILYNX_API_H

#include <QObject>
#include <QString>
#include <QStringList>
#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QJsonObject>
#include <QJsonArray>
#include <QJsonDocument>

class WikiLynxAPI : public QObject {
    Q_OBJECT

public:
    explicit WikiLynxAPI(const QString &serverUrl = "http://localhost:8080", QObject *parent = nullptr);

    // 1. Fetch available league names for the client dropdown
    void fetchLeagues();

    // 2. Submit score when player finishes a level
    // NOTE: All parameters except league are strictly required by the server!
    void submitScore(const QString &level,
                     const QString &userId,
                     const QString &username,
                     double timeTakenSeconds,
                     int clicksCount,
                     double difficultyMultiplier, // strictly 0.0 to 1.0 (e.g. 0.25, 0.50, 0.75, 1.0)
                     int status,                  // 1 = Win, 0 = Lose
                     int checkpoints = 0,         // pass 0 if none
                     const QString &league = "Global Championship");

signals:
    void leaguesReceived(const QStringList &leagues);
    void leaguesFailed(const QString &error);
    void scoreRecorded(int rank, double score, const QJsonObject &data);
    void scoreFailed(const QString &error);

private:
    QString baseUrl;
    QNetworkAccessManager *networkManager;
};

#endif // WIKILYNX_API_H
```

### Implementation File (`wikilynx_api.cpp`)

```cpp
#include "wikilynx_api.h"
#include <QUrl>
#include <QNetworkRequest>

WikiLynxAPI::WikiLynxAPI(const QString &serverUrl, QObject *parent)
    : QObject(parent), baseUrl(serverUrl), networkManager(new QNetworkAccessManager(this)) {}

void WikiLynxAPI::fetchLeagues() {
    QUrl url(baseUrl + "/api/leagues");
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");

    QNetworkReply *reply = networkManager->get(request);

    connect(reply, &QNetworkReply::finished, this, [this, reply]() {
        reply->deleteLater();
        if (reply->error() == QNetworkReply::NoError) {
            QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
            QStringList leagues;
            if (doc.isArray()) {
                for (const QJsonValue &val : doc.array()) {
                    leagues.append(val.toString());
                }
            }
            emit leaguesReceived(leagues);
        } else {
            emit leaguesFailed(reply->errorString());
        }
    });
}

void WikiLynxAPI::submitScore(const QString &level,
                              const QString &userId,
                              const QString &username,
                              double timeTakenSeconds,
                              int clicksCount,
                              double difficultyMultiplier,
                              int status,
                              int checkpoints,
                              const QString &league) 
{
    QUrl url(baseUrl + "/api/score");
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");

    QJsonObject payload;
    payload["level"] = level;
    payload["userId"] = userId;
    payload["username"] = username;
    payload["time"] = timeTakenSeconds;
    payload["clicks"] = clicksCount;
    payload["difficulty"] = difficultyMultiplier; // 0.0 to 1.0
    payload["status"] = status;                 // 1 for Win, 0 for Lose
    payload["checkpoints"] = checkpoints;       // pass 0 if none
    payload["league"] = league.isEmpty() ? "Global Championship" : league;

    QByteArray data = QJsonDocument(payload).toJson();
    QNetworkReply *reply = networkManager->post(request, data);

    connect(reply, &QNetworkReply::finished, this, [this, reply]() {
        reply->deleteLater();
        if (reply->error() == QNetworkReply::NoError) {
            QJsonDocument doc = QJsonDocument::fromJson(reply->readAll());
            QJsonObject obj = doc.object();
            int rank = obj.value("rank").toInt();
            double score = obj.value("score").toDouble();
            emit scoreRecorded(rank, score, obj.value("data").toObject());
        } else {
            emit scoreFailed(reply->errorString());
        }
    });
}
```

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
