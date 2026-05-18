interface Playlist {
  ID: string;
  Title: string;
  ItemCount: number;
}

interface QuotaInfo {
  used: number;
  limit: number;
  remaining: number;
  date: string;
}

interface JobResponse {
  jobId: string;
  status: string;
}

interface JobStatus {
  status: string;
  progress: number;
  total: number;
  done: number;
  newPlaylistId?: string;
  error?: string;
}

interface RandomizeRequest {
  playlistId: string;
  newName: string;
}

interface ApiError {
  error: string;
}

const QuotaCostList = 1;
const QuotaCostCreate = 50;
const QuotaCostInsert = 50;

let playlists: Playlist[] = [];
let selectedPlaylistId: string | null = null;
let selectedPlaylistCount = 0;
let pollInterval: ReturnType<typeof setInterval> | null = null;
let quotaInfo: QuotaInfo | null = null;

async function fetchQuota(): Promise<void> {
  try {
    const res = await fetch("/api/quota");
    if (res.ok) {
      quotaInfo = await res.json() as QuotaInfo;
      updateQuotaDisplay();
    }
  } catch {
  }
}

function updateQuotaDisplay(): void {
  if (!quotaInfo) return;
  const pct =
    quotaInfo.limit > 0
      ? Math.min((quotaInfo.used / quotaInfo.limit) * 100, 100)
      : 0;
  const quotaText = document.getElementById("quota-text");
  if (quotaText) {
    quotaText.textContent =
      `Quota: ${quotaInfo.used} / ${quotaInfo.limit} used (${quotaInfo.remaining} remaining)`;
  }
  const fill = document.getElementById("quota-fill");
  if (fill) {
    fill.style.width = `${pct}%`;
    fill.className = `quota-fill${pct > 80 ? " quota-critical" : pct > 50 ? " quota-warning" : ""}`;
  }
}

function estimateQuotaCost(itemCount: number | undefined): number {
  if (!itemCount || itemCount === 0) return QuotaCostCreate;
  return QuotaCostCreate + itemCount * QuotaCostInsert;
}

async function fetchPlaylists(): Promise<void> {
  try {
    const res = await fetch("/api/playlists");
    if (!res.ok) {
      const err = await res.json() as ApiError;
      throw new Error(err.error || "Failed to fetch playlists");
    }
    playlists = await res.json() as Playlist[];
    const loadingEl = document.getElementById("loading");
    if (loadingEl) loadingEl.classList.add("hidden");

    if (playlists.length === 0) {
      const noPl = document.getElementById("no-playlists");
      if (noPl) noPl.classList.remove("hidden");
    } else {
      const plEl = document.getElementById("playlists");
      if (plEl) plEl.classList.remove("hidden");
      renderPlaylists(playlists);
    }
  } catch (err: unknown) {
    const loadingEl = document.getElementById("loading");
    if (loadingEl) loadingEl.classList.add("hidden");
    showError(err instanceof Error ? err.message : String(err));
  }
}

function renderPlaylists(list: Playlist[]): void {
  const container = document.getElementById("playlist-list");
  if (!container) return;
  container.innerHTML = "";
  if (list.length === 0) {
    container.innerHTML = '<p class="no-results">No playlists match your filter.</p>';
    return;
  }
  for (const pl of list) {
    const card = document.createElement("div");
    card.className = "playlist-card";
    const cost = estimateQuotaCost(pl.ItemCount);
    const insufficient = quotaInfo && quotaInfo.remaining < cost;
    const btnDisabled = insufficient ? " btn-disabled" : "";
    const btnDisabledAttr = insufficient ? "disabled" : "";
    const btnOnClick = insufficient
      ? ""
      : `openModal('${pl.ID}', '${escapeHtml(pl.Title)}', ${pl.ItemCount || 0})`;
    card.innerHTML = `
      <div class="playlist-info">
        <span class="playlist-title">${escapeHtml(pl.Title)}</span>
        <span class="playlist-count">${pl.ItemCount || "?"} videos &middot; ~${cost} quota</span>
      </div>
      <button class="btn btn-randomize${btnDisabled}"
        onclick="${btnOnClick}"
        ${btnDisabledAttr}>
        ${insufficient ? "Insufficient Quota" : "Randomize"}
      </button>
    `;
    container.appendChild(card);
  }
}

function filterPlaylists(): void {
  const q = (document.getElementById("search") as HTMLInputElement | null)?.value.toLowerCase() ?? "";
  const filtered = playlists.filter((pl) =>
    pl.Title.toLowerCase().includes(q)
  );
  renderPlaylists(filtered);
}

function openModal(id: string, title: string, itemCount: number): void {
  selectedPlaylistId = id;
  selectedPlaylistCount = itemCount;
  const nameEl = document.getElementById("modal-playlist-name");
  if (nameEl) nameEl.textContent = title;

  const cost = estimateQuotaCost(itemCount);
  const costEl = document.getElementById("modal-quota-cost");
  if (costEl && quotaInfo) {
    costEl.textContent =
      `Estimated quota cost: ${cost} units (${quotaInfo.remaining >= cost ? "Sufficient" : "Insufficient"} remaining)`;
    costEl.className = `quota-cost ${quotaInfo.remaining >= cost ? "quota-ok" : "quota-low"}`;
  }

  const now = new Date();
  const monthYear = now.toLocaleString("default", { month: "long", year: "numeric" });
  const nameInput = document.getElementById("new-name") as HTMLInputElement | null;
  if (nameInput) {
    nameInput.value = `${title}-randomized-${monthYear}`;
  }

  const modal = document.getElementById("modal");
  if (modal) modal.classList.remove("hidden");
  nameInput?.focus();
}

function closeModal(): void {
  const modal = document.getElementById("modal");
  if (modal) modal.classList.add("hidden");
  selectedPlaylistId = null;
}

function closeProgressModal(): void {
  const modal = document.getElementById("progress-modal");
  if (modal) modal.classList.add("hidden");
  const fill = document.getElementById("progress-fill");
  if (fill) fill.style.width = "0%";
  const text = document.getElementById("progress-text");
  if (text) text.textContent = "Starting...";
  const result = document.getElementById("progress-result");
  if (result) result.classList.add("hidden");
  const paused = document.getElementById("progress-paused");
  if (paused) paused.classList.add("hidden");
  const err = document.getElementById("progress-error");
  if (err) err.classList.add("hidden");
  if (pollInterval) {
    clearInterval(pollInterval);
    pollInterval = null;
  }
}

function showError(msg: string): void {
  const el = document.getElementById("error");
  if (el) {
    el.textContent = msg;
    el.classList.remove("hidden");
  }
}

async function startRandomize(): Promise<void> {
  const nameInput = document.getElementById("new-name") as HTMLInputElement | null;
  const name = nameInput?.value.trim() ?? "";
  if (!name) {
    alert("Please enter a name for the new playlist.");
    return;
  }
  const id = selectedPlaylistId;
  closeModal();

  const progressModal = document.getElementById("progress-modal");
  if (progressModal) progressModal.classList.remove("hidden");
  const progressText = document.getElementById("progress-text");
  if (progressText) progressText.textContent = "Starting...";
  const fill = document.getElementById("progress-fill");
  if (fill) fill.style.width = "0%";
  const result = document.getElementById("progress-result");
  if (result) result.classList.add("hidden");
  const paused = document.getElementById("progress-paused");
  if (paused) paused.classList.add("hidden");
  const err = document.getElementById("progress-error");
  if (err) err.classList.add("hidden");

  try {
    const res = await fetch("/api/randomize", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ playlistId: id, newName: name } as RandomizeRequest),
    });
    if (!res.ok) {
      const errData = await res.json() as ApiError;
      throw new Error(errData.error || "Failed to start randomization");
    }
    const job = await res.json() as JobResponse;
    startPolling(job.jobId);
  } catch (err: unknown) {
    if (progressModal) progressModal.classList.add("hidden");
    showError(err instanceof Error ? err.message : String(err));
  }
}

function startPolling(jobId: string): void {
  if (pollInterval) clearInterval(pollInterval);
  pollInterval = setInterval(async () => {
    try {
      const res = await fetch(`/api/jobs/${jobId}`);
      if (!res.ok) {
        if (pollInterval) clearInterval(pollInterval);
        pollInterval = null;
        throw new Error("Failed to check job status");
      }
      const status = await res.json() as JobStatus;
      updateProgress(status);
      fetchQuota();
    } catch (err: unknown) {
      if (pollInterval) clearInterval(pollInterval);
      pollInterval = null;
      const text = document.getElementById("progress-text");
      if (text) text.textContent = "Error checking status";
      const errEl = document.getElementById("progress-error");
      if (errEl) {
        errEl.textContent = err instanceof Error ? err.message : String(err);
        errEl.classList.remove("hidden");
      }
    }
  }, 1500);
}

function updateProgress(status: JobStatus): void {
  const fill = document.getElementById("progress-fill");
  const text = document.getElementById("progress-text");

  switch (status.status) {
    case "pending":
      if (text) text.textContent = "Waiting to start...";
      break;
    case "fetching":
      if (text) text.textContent = "Fetching playlist items...";
      if (fill) fill.style.width = "25%";
      break;
    case "shuffling":
      if (text) text.textContent = "Shuffling items...";
      if (fill) fill.style.width = "50%";
      break;
    case "inserting": {
      const pct =
        status.total > 0
          ? Math.round((status.done / status.total) * 50) + 50
          : 50;
      if (fill) fill.style.width = `${Math.min(pct, 99)}%`;
      if (text)
        text.textContent = `Inserting items... ${status.done} / ${status.total}`;
      break;
    }
    case "done": {
      if (fill) fill.style.width = "100%";
      const link = document.getElementById("playlist-link") as HTMLAnchorElement | null;
      if (link && status.newPlaylistId) {
        link.href = `https://www.youtube.com/playlist?list=${status.newPlaylistId}`;
      }
      const result = document.getElementById("progress-result");
      if (result) result.classList.remove("hidden");
      if (text) text.textContent = "Playlist created successfully!";
      if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
      }
      break;
    }
    case "paused": {
      const pct =
        status.total > 0
          ? Math.round((status.done / status.total) * 100)
          : 0;
      if (fill) fill.style.width = `${pct}%`;
      if (text)
        text.textContent = `Inserted ${status.done} / ${status.total} items`;
      const paused = document.getElementById("progress-paused");
      if (paused) paused.classList.remove("hidden");
      if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
      }
      break;
    }
    case "error": {
      if (fill) fill.style.width = "100%";
      if (text) text.textContent = "Error";
      const errEl = document.getElementById("progress-error");
      if (errEl) {
        errEl.textContent = status.error || "An unknown error occurred";
        errEl.classList.remove("hidden");
      }
      if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
      }
      break;
    }
  }
}

function escapeHtml(str: string): string {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

(window as unknown as Record<string, unknown>).filterPlaylists = filterPlaylists;
(window as unknown as Record<string, unknown>).closeModal = closeModal;
(window as unknown as Record<string, unknown>).startRandomize = startRandomize;
(window as unknown as Record<string, unknown>).closeProgressModal = closeProgressModal;
(window as unknown as Record<string, unknown>).openModal = openModal;

fetchQuota();
fetchPlaylists();
