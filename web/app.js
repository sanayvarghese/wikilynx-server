/**
 * WikiLYNX Leaderboard & League Web Application
 */

const state = {
  viewType: 'level', // 'league' or 'level'
  currentSelection: '',
  leagues: [],
  levels: [],
  scores: [],
  filteredScores: [],
  autoRefreshTimer: null,
  isRefreshing: false
};

// DOM Elements
const levelSelect = document.getElementById('levelSelect');
const playerSearch = document.getElementById('playerSearch');
const clearSearch = document.getElementById('clearSearch');
const refreshBtn = document.getElementById('refreshBtn');
const autoRefreshToggle = document.getElementById('autoRefreshToggle');
const tableHead = document.getElementById('tableHead');
const leaderboardBody = document.getElementById('leaderboardBody');
const emptyState = document.getElementById('emptyState');
const entriesCount = document.getElementById('entriesCount');
const lastUpdated = document.getElementById('lastUpdated');
const tableHeading = document.getElementById('tableHeading');
const typePill = document.getElementById('typePill');

// Summary Card Elements
const topRecordLabel = document.getElementById('topRecordLabel');
const topRecordScore = document.getElementById('topRecordScore');
const topRecordPlayer = document.getElementById('topRecordPlayer');
const totalPlayersCount = document.getElementById('totalPlayersCount');
const totalPlayersSub = document.getElementById('totalPlayersSub');
const card3Label = document.getElementById('card3Label');
const card3Value = document.getElementById('card3Value');
const card3Sub = document.getElementById('card3Sub');
const activeLevelBadge = document.getElementById('activeLevelBadge');
const activeLevelSub = document.getElementById('activeLevelSub');

// Initialize
document.addEventListener('DOMContentLoaded', () => {
  initEventListeners();
  loadAllCategories();
  setupAutoRefresh();
});

function initEventListeners() {
  levelSelect.addEventListener('change', (e) => {
    const val = e.target.value;
    handleSelectionChange(val);
  });

  playerSearch.addEventListener('input', (e) => {
    const query = e.target.value.trim().toLowerCase();
    clearSearch.style.display = query ? 'block' : 'none';
    filterScores(query);
  });

  clearSearch.addEventListener('click', () => {
    playerSearch.value = '';
    clearSearch.style.display = 'none';
    filterScores('');
  });

  refreshBtn.addEventListener('click', () => {
    refreshAll();
  });

  autoRefreshToggle.addEventListener('change', () => {
    setupAutoRefresh();
  });
}

function handleSelectionChange(val) {
  if (val.startsWith('league:')) {
    state.viewType = 'league';
    state.currentSelection = val.replace('league:', '');
    updateUrlParams('league', state.currentSelection);
  } else if (val.startsWith('level:')) {
    state.viewType = 'level';
    state.currentSelection = val.replace('level:', '');
    updateUrlParams('level', state.currentSelection);
  }
  loadLeaderboard();
}

function updateUrlParams(type, name) {
  const url = new URL(window.location);
  url.searchParams.delete('league');
  url.searchParams.delete('level');
  if (type && name) {
    url.searchParams.set(type, name);
  }
  window.history.replaceState({}, '', url);
}

async function loadAllCategories() {
  try {
    const [leaguesRes, levelsRes] = await Promise.all([
      fetch('/api/leagues'),
      fetch('/api/levels')
    ]);

    if (leaguesRes.ok) {
      const rawLeagues = await leaguesRes.json() || [];
      state.leagues = rawLeagues.map(l => typeof l === 'string' ? { name: l } : l);
    }
    if (levelsRes.ok) {
      state.levels = await levelsRes.json() || [];
    }

    renderDropdown();
    resolveInitialSelection();
    loadLeaderboard();
  } catch (err) {
    console.error('Error loading leagues & levels:', err);
    levelSelect.innerHTML = `<option disabled selected>Failed to load data</option>`;
  }
}

function renderDropdown() {
  let html = '';

  if (state.leagues.length > 0) {
    html += `<optgroup label="🏆 Competitive Leagues">`;
    state.leagues.forEach(l => {
      const name = typeof l === 'string' ? l : l.name;
      const count = (l.totalPlayers !== undefined && l.totalPlayers > 0) ? ` (${l.totalPlayers} players)` : '';
      html += `<option value="league:${escapeHtml(name)}">🏆 ${escapeHtml(name)}${count}</option>`;
    });
    html += `</optgroup>`;
  }

  if (state.levels.length > 0) {
    html += `<optgroup label="🎮 Individual Levels">`;
    state.levels.forEach(lvl => {
      const count = lvl.totalScores === 1 ? ' (1 run)' : (lvl.totalScores > 0 ? ` (${lvl.totalScores} runs)` : '');
      html += `<option value="level:${escapeHtml(lvl.name)}">🎮 ${escapeHtml(lvl.name)}${count}</option>`;
    });
    html += `</optgroup>`;
  }

  if (state.leagues.length === 0 && state.levels.length === 0) {
    html = `<option disabled selected>No leagues or levels registered</option>`;
  }

  levelSelect.innerHTML = html;
}

function resolveInitialSelection() {
  const urlParams = new URLSearchParams(window.location.search);
  const leagueParam = urlParams.get('league');
  const levelParam = urlParams.get('level');

  if (leagueParam && state.leagues.some(l => l.name === leagueParam)) {
    state.viewType = 'league';
    state.currentSelection = leagueParam;
    levelSelect.value = `league:${leagueParam}`;
    return;
  }

  if (levelParam && state.levels.some(l => l.name === levelParam)) {
    state.viewType = 'level';
    state.currentSelection = levelParam;
    levelSelect.value = `level:${levelParam}`;
    return;
  }

  // Default priority: Global Championship league or first available league/level
  if (state.leagues.length > 0) {
    const globalLeague = state.leagues.find(l => l.name === 'Global Championship') || state.leagues[0];
    state.viewType = 'league';
    state.currentSelection = globalLeague.name;
    levelSelect.value = `league:${globalLeague.name}`;
    updateUrlParams('league', globalLeague.name);
  } else if (state.levels.length > 0) {
    state.viewType = 'level';
    state.currentSelection = state.levels[0].name;
    levelSelect.value = `level:${state.levels[0].name}`;
    updateUrlParams('level', state.levels[0].name);
  }
}

async function loadLeaderboard() {
  if (!state.currentSelection) return;

  const isLeague = state.viewType === 'league';
  const endpoint = isLeague 
    ? `/api/leaderboard?league=${encodeURIComponent(state.currentSelection)}`
    : `/api/leaderboard?level=${encodeURIComponent(state.currentSelection)}`;

  try {
    const res = await fetch(endpoint);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();

    state.scores = data.scores || [];
    state.filteredScores = [...state.scores];

    updateHeaderAndCards(data);
    renderTableHead();
    renderTableBody(state.filteredScores);
    updateLastUpdatedTime();
  } catch (err) {
    console.error('Error fetching leaderboard:', err);
    leaderboardBody.innerHTML = `
      <tr>
        <td colspan="8" class="loading-state" style="color: #ef4444;">
          Failed to load data: ${escapeHtml(err.message)}
        </td>
      </tr>`;
  }
}

function updateHeaderAndCards(data) {
  const isLeague = state.viewType === 'league';

  if (isLeague) {
    typePill.textContent = 'LEAGUE';
    typePill.className = 'type-pill pill-league';
    tableHeading.textContent = `League Standings: "${state.currentSelection}"`;
    activeLevelBadge.textContent = state.currentSelection;
    activeLevelSub.textContent = 'Cumulative Cross-Level League';

    topRecordLabel.textContent = 'Leader Score';
    totalPlayersSub.textContent = 'In this league';
    card3Label.textContent = 'Combined Runs';

    if (state.scores.length > 0) {
      const top = state.scores[0];
      topRecordScore.textContent = `${Number(top.totalScore).toLocaleString()} pts`;
      topRecordPlayer.textContent = `by ${top.username}`;
      totalPlayersCount.textContent = state.scores.length;

      const totalLevelsCleared = state.scores.reduce((sum, s) => sum + s.levelsCleared, 0);
      card3Value.textContent = totalLevelsCleared;
      card3Sub.textContent = 'Levels conquered';
    } else {
      topRecordScore.textContent = '--';
      topRecordPlayer.textContent = 'No records yet';
      totalPlayersCount.textContent = '0';
      card3Value.textContent = '0';
      card3Sub.textContent = 'Levels conquered';
    }
  } else {
    typePill.textContent = 'LEVEL';
    typePill.className = 'type-pill pill-level';
    tableHeading.textContent = `Level Rankings: "${state.currentSelection}"`;
    activeLevelBadge.textContent = state.currentSelection;
    activeLevelSub.textContent = 'Single Level Leaderboard';

    topRecordLabel.textContent = 'Top Run Score';
    totalPlayersSub.textContent = 'On this level';
    card3Label.textContent = 'Min Clicks';

    if (state.scores.length > 0) {
      const top = state.scores[0];
      topRecordScore.textContent = `${Number(top.score || 0).toLocaleString()} pts`;
      topRecordPlayer.textContent = `by ${top.username} (${formatTime(top.timeTaken)})`;
      totalPlayersCount.textContent = state.scores.length;

      const bestClicks = state.scores.reduce((min, s) => s.clicks < min.clicks ? s : min, state.scores[0]);
      card3Value.textContent = `${bestClicks.clicks} clicks`;
      card3Sub.textContent = `by ${bestClicks.username}`;
    } else {
      topRecordScore.textContent = '--';
      topRecordPlayer.textContent = 'No records yet';
      totalPlayersCount.textContent = '0';
      card3Value.textContent = '--';
      card3Sub.textContent = 'Best route';
    }
  }
}

function renderTableHead() {
  const isLeague = state.viewType === 'league';

  if (isLeague) {
    tableHead.innerHTML = `
      <tr>
        <th class="col-rank">Rank</th>
        <th class="col-player">Player</th>
        <th class="col-score">League Score</th>
        <th class="col-chk">Levels Cleared</th>
        <th class="col-time">Combined Time</th>
        <th class="col-clicks">Total Clicks</th>
        <th class="col-date">Last Active</th>
      </tr>
    `;
  } else {
    tableHead.innerHTML = `
      <tr>
        <th class="col-rank">Rank</th>
        <th class="col-player">Player</th>
        <th class="col-score">Score</th>
        <th class="col-time">Time</th>
        <th class="col-clicks">Clicks</th>
        <th>Difficulty</th>
        <th class="col-chk">Checkpoints</th>
        <th class="col-status">Status</th>
        <th class="col-date">Date</th>
      </tr>
    `;
  }
}

function renderTableBody(scores) {
  entriesCount.textContent = `${scores.length} player${scores.length === 1 ? '' : 's'}`;

  if (scores.length === 0) {
    leaderboardBody.innerHTML = '';
    emptyState.style.display = 'block';
    return;
  }

  emptyState.style.display = 'none';
  const isLeague = state.viewType === 'league';

  if (isLeague) {
    leaderboardBody.innerHTML = scores.map(entry => {
      const rankBadge = getRankBadge(entry.rank);
      const shortUserId = entry.userId ? entry.userId.slice(0, 8) : 'unknown';
      const relativeDate = formatRelativeTime(entry.lastActive);
      const formattedTime = formatTime(entry.totalTime);

      return `
        <tr>
          <td class="col-rank">${rankBadge}</td>
          <td class="col-player">
            <div class="player-cell">
              <span class="player-name">${escapeHtml(entry.username || 'Anonymous')}</span>
              <span class="player-id" title="User ID: ${escapeHtml(entry.userId)}">id: ${escapeHtml(shortUserId)}</span>
            </div>
          </td>
          <td class="col-score">
            <span class="score-badge">
              <span class="score-star">★</span> ${Number(entry.totalScore).toLocaleString()}
            </span>
          </td>
          <td class="col-chk">
            <span class="mono-cell" style="font-weight:700; color:var(--text-main);">${entry.levelsCleared}</span>
          </td>
          <td class="col-time">
            <span class="time-cell">${formattedTime}</span>
          </td>
          <td class="col-clicks">
            <span class="mono-cell">${entry.totalClicks}</span>
          </td>
          <td class="col-date">
            <span class="mono-cell" title="${escapeHtml(entry.lastActive)}">${relativeDate}</span>
          </td>
        </tr>
      `;
    }).join('');
  } else {
    leaderboardBody.innerHTML = scores.map(entry => {
      const rankBadge = getRankBadge(entry.rank);
      const shortUserId = entry.userId ? entry.userId.slice(0, 8) : 'unknown';
      const isWin = entry.status === 1 || entry.status === '1' || entry.status === true;
      const statusClass = isWin ? 'status-win' : 'status-lose';
      const statusText = isWin ? 'Win (1)' : 'Loss (0)';
      const diffClass = getDiffClass(entry.difficulty);

      return `
        <tr>
          <td class="col-rank">${rankBadge}</td>
          <td class="col-player">
            <div class="player-cell">
              <span class="player-name">${escapeHtml(entry.username || 'Anonymous')}</span>
              <span class="player-id" title="User ID: ${escapeHtml(entry.userId)}">id: ${escapeHtml(shortUserId)}</span>
            </div>
          </td>
          <td class="col-score">
            <span class="score-badge">
              <span class="score-star">★</span> ${Number(entry.score || 0).toLocaleString()}
            </span>
          </td>
          <td class="col-time">
            <span class="time-cell">⏱️ ${formattedTime}</span>
          </td>
          <td class="col-clicks">
            <span class="mono-cell">${entry.clicks}</span>
          </td>
          <td>
            <span class="diff-badge ${diffClass}">${escapeHtml(entry.difficulty || 'easy')}</span>
          </td>
          <td class="col-chk">
            <span class="mono-cell">${entry.checkpoints || 0}</span>
          </td>
          <td class="col-status">
            <span class="status-badge ${statusClass}">${statusText}</span>
          </td>
          <td class="col-date">
            <span class="mono-cell" title="${escapeHtml(entry.submittedAt)}">${relativeDate}</span>
          </td>
        </tr>
      `;
    }).join('');
  }
}

function filterScores(query) {
  if (!query) {
    state.filteredScores = [...state.scores];
  } else {
    state.filteredScores = state.scores.filter(entry => 
      (entry.username && entry.username.toLowerCase().includes(query)) ||
      (entry.userId && entry.userId.toLowerCase().includes(query))
    );
  }
  renderTableBody(state.filteredScores);
}

function getRankBadge(rank) {
  if (rank === 1) {
    return `<span class="rank-badge rank-gold" title="1st Place - Gold">🥇</span>`;
  } else if (rank === 2) {
    return `<span class="rank-badge rank-silver" title="2nd Place - Silver">🥈</span>`;
  } else if (rank === 3) {
    return `<span class="rank-badge rank-bronze" title="3rd Place - Bronze">🥉</span>`;
  }
  return `<span class="rank-badge rank-default">#${rank}</span>`;
}

function getDiffClass(diff) {
  if (!diff) return 'diff-easy';
  const d = diff.toLowerCase();
  if (d.includes('hard')) return 'diff-hard';
  if (d.includes('medium')) return 'diff-medium';
  if (d.includes('insane') || d.includes('expert')) return 'diff-insane';
  return 'diff-easy';
}

function formatTime(seconds) {
  if (typeof seconds !== 'number' || isNaN(seconds)) return '--';
  const mins = Math.floor(seconds / 60);
  const secs = (seconds % 60).toFixed(2);
  if (mins > 0) {
    return `${mins}m ${secs.padStart(5, '0')}s`;
  }
  return `${secs}s`;
}

function formatRelativeTime(dateString) {
  if (!dateString) return '--';
  const date = new Date(dateString);
  const now = new Date();
  const diffSec = Math.floor((now - date) / 1000);

  if (diffSec < 60) return 'Just now';
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays}d ago`;

  return date.toLocaleDateString();
}

function updateLastUpdatedTime() {
  const now = new Date();
  lastUpdated.textContent = `Updated: ${now.toLocaleTimeString()}`;
}

function setupAutoRefresh() {
  if (state.autoRefreshTimer) {
    clearInterval(state.autoRefreshTimer);
    state.autoRefreshTimer = null;
  }

  if (autoRefreshToggle.checked) {
    state.autoRefreshTimer = setInterval(() => {
      refreshAll(true);
    }, 10000);
  }
}

async function refreshAll(isBackground = false) {
  if (state.isRefreshing) return;
  state.isRefreshing = true;

  if (!isBackground) {
    refreshBtn.style.transform = 'rotate(180deg)';
  }

  try {
    const [leaguesRes, levelsRes] = await Promise.all([
      fetch('/api/leagues'),
      fetch('/api/levels')
    ]);

    if (leaguesRes.ok) state.leagues = await leaguesRes.json() || [];
    if (levelsRes.ok) state.levels = await levelsRes.json() || [];

    renderDropdown();
    if (state.currentSelection) {
      levelSelect.value = `${state.viewType}:${state.currentSelection}`;
      await loadLeaderboard();
    }
  } catch (err) {
    console.error('Refresh error:', err);
  } finally {
    state.isRefreshing = false;
    setTimeout(() => {
      refreshBtn.style.transform = '';
    }, 300);
  }
}

function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}
