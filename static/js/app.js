"use strict";
const QuotaCostList = 1;
const QuotaCostCreate = 50;
const QuotaCostInsert = 50;
let playlists = [];
let selectedPlaylistId = null;
let selectedPlaylistCount = 0;
let pollInterval = null;
let quotaInfo = null;
async function fetchQuota() {
    try {
        const res = await fetch("/api/quota");
        if (res.ok) {
            quotaInfo = await res.json();
            updateQuotaDisplay();
        }
    }
    catch {
    }
}
function updateQuotaDisplay() {
    if (!quotaInfo)
        return;
    const pct = quotaInfo.limit > 0
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
function estimateQuotaCost(itemCount) {
    if (!itemCount || itemCount === 0)
        return QuotaCostCreate;
    return QuotaCostCreate + itemCount * QuotaCostInsert;
}
async function fetchPlaylists() {
    try {
        const res = await fetch("/api/playlists");
        if (!res.ok) {
            const err = await res.json();
            throw new Error(err.error || "Failed to fetch playlists");
        }
        playlists = await res.json();
        const loadingEl = document.getElementById("loading");
        if (loadingEl)
            loadingEl.classList.add("hidden");
        if (playlists.length === 0) {
            const noPl = document.getElementById("no-playlists");
            if (noPl)
                noPl.classList.remove("hidden");
        }
        else {
            const plEl = document.getElementById("playlists");
            if (plEl)
                plEl.classList.remove("hidden");
            renderPlaylists(playlists);
        }
    }
    catch (err) {
        const loadingEl = document.getElementById("loading");
        if (loadingEl)
            loadingEl.classList.add("hidden");
        showError(err instanceof Error ? err.message : String(err));
    }
}
function renderPlaylists(list) {
    const container = document.getElementById("playlist-list");
    if (!container)
        return;
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
function filterPlaylists() {
    const q = document.getElementById("search")?.value.toLowerCase() ?? "";
    const filtered = playlists.filter((pl) => pl.Title.toLowerCase().includes(q));
    renderPlaylists(filtered);
}
function openModal(id, title, itemCount) {
    selectedPlaylistId = id;
    selectedPlaylistCount = itemCount;
    const nameEl = document.getElementById("modal-playlist-name");
    if (nameEl)
        nameEl.textContent = title;
    const cost = estimateQuotaCost(itemCount);
    const costEl = document.getElementById("modal-quota-cost");
    if (costEl && quotaInfo) {
        costEl.textContent =
            `Estimated quota cost: ${cost} units (${quotaInfo.remaining >= cost ? "Sufficient" : "Insufficient"} remaining)`;
        costEl.className = `quota-cost ${quotaInfo.remaining >= cost ? "quota-ok" : "quota-low"}`;
    }
    const now = new Date();
    const monthYear = now.toLocaleString("default", { month: "long", year: "numeric" });
    const nameInput = document.getElementById("new-name");
    if (nameInput) {
        nameInput.value = `${title}-randomized-${monthYear}`;
    }
    const modal = document.getElementById("modal");
    if (modal)
        modal.classList.remove("hidden");
    nameInput?.focus();
}
function closeModal() {
    const modal = document.getElementById("modal");
    if (modal)
        modal.classList.add("hidden");
    selectedPlaylistId = null;
}
function closeProgressModal() {
    const modal = document.getElementById("progress-modal");
    if (modal)
        modal.classList.add("hidden");
    const fill = document.getElementById("progress-fill");
    if (fill)
        fill.style.width = "0%";
    const text = document.getElementById("progress-text");
    if (text)
        text.textContent = "Starting...";
    const result = document.getElementById("progress-result");
    if (result)
        result.classList.add("hidden");
    const paused = document.getElementById("progress-paused");
    if (paused)
        paused.classList.add("hidden");
    const err = document.getElementById("progress-error");
    if (err)
        err.classList.add("hidden");
    if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
    }
}
function showError(msg) {
    const el = document.getElementById("error");
    if (el) {
        el.textContent = msg;
        el.classList.remove("hidden");
    }
}
async function startRandomize() {
    const nameInput = document.getElementById("new-name");
    const name = nameInput?.value.trim() ?? "";
    if (!name) {
        alert("Please enter a name for the new playlist.");
        return;
    }
    const id = selectedPlaylistId;
    closeModal();
    const progressModal = document.getElementById("progress-modal");
    if (progressModal)
        progressModal.classList.remove("hidden");
    const progressText = document.getElementById("progress-text");
    if (progressText)
        progressText.textContent = "Starting...";
    const fill = document.getElementById("progress-fill");
    if (fill)
        fill.style.width = "0%";
    const result = document.getElementById("progress-result");
    if (result)
        result.classList.add("hidden");
    const paused = document.getElementById("progress-paused");
    if (paused)
        paused.classList.add("hidden");
    const err = document.getElementById("progress-error");
    if (err)
        err.classList.add("hidden");
    try {
        const res = await fetch("/api/randomize", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ playlistId: id, newName: name }),
        });
        if (!res.ok) {
            const errData = await res.json();
            throw new Error(errData.error || "Failed to start randomization");
        }
        const job = await res.json();
        startPolling(job.jobId);
    }
    catch (err) {
        if (progressModal)
            progressModal.classList.add("hidden");
        showError(err instanceof Error ? err.message : String(err));
    }
}
function startPolling(jobId) {
    if (pollInterval)
        clearInterval(pollInterval);
    pollInterval = setInterval(async () => {
        try {
            const res = await fetch(`/api/jobs/${jobId}`);
            if (!res.ok) {
                if (pollInterval)
                    clearInterval(pollInterval);
                pollInterval = null;
                throw new Error("Failed to check job status");
            }
            const status = await res.json();
            updateProgress(status);
            fetchQuota();
        }
        catch (err) {
            if (pollInterval)
                clearInterval(pollInterval);
            pollInterval = null;
            const text = document.getElementById("progress-text");
            if (text)
                text.textContent = "Error checking status";
            const errEl = document.getElementById("progress-error");
            if (errEl) {
                errEl.textContent = err instanceof Error ? err.message : String(err);
                errEl.classList.remove("hidden");
            }
        }
    }, 1500);
}
function updateProgress(status) {
    const fill = document.getElementById("progress-fill");
    const text = document.getElementById("progress-text");
    switch (status.status) {
        case "pending":
            if (text)
                text.textContent = "Waiting to start...";
            break;
        case "fetching":
            if (text)
                text.textContent = "Fetching playlist items...";
            if (fill)
                fill.style.width = "25%";
            break;
        case "shuffling":
            if (text)
                text.textContent = "Shuffling items...";
            if (fill)
                fill.style.width = "50%";
            break;
        case "inserting": {
            const pct = status.total > 0
                ? Math.round((status.done / status.total) * 50) + 50
                : 50;
            if (fill)
                fill.style.width = `${Math.min(pct, 99)}%`;
            if (text)
                text.textContent = `Inserting items... ${status.done} / ${status.total}`;
            break;
        }
        case "done": {
            if (fill)
                fill.style.width = "100%";
            const link = document.getElementById("playlist-link");
            if (link && status.newPlaylistId) {
                link.href = `https://www.youtube.com/playlist?list=${status.newPlaylistId}`;
            }
            const result = document.getElementById("progress-result");
            if (result)
                result.classList.remove("hidden");
            if (text)
                text.textContent = "Playlist created successfully!";
            if (pollInterval) {
                clearInterval(pollInterval);
                pollInterval = null;
            }
            break;
        }
        case "paused": {
            const pct = status.total > 0
                ? Math.round((status.done / status.total) * 100)
                : 0;
            if (fill)
                fill.style.width = `${pct}%`;
            if (text)
                text.textContent = `Inserted ${status.done} / ${status.total} items`;
            const paused = document.getElementById("progress-paused");
            if (paused)
                paused.classList.remove("hidden");
            if (pollInterval) {
                clearInterval(pollInterval);
                pollInterval = null;
            }
            break;
        }
        case "error": {
            if (fill)
                fill.style.width = "100%";
            if (text)
                text.textContent = "Error";
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
function escapeHtml(str) {
    const div = document.createElement("div");
    div.textContent = str;
    return div.innerHTML;
}
window.filterPlaylists = filterPlaylists;
window.closeModal = closeModal;
window.startRandomize = startRandomize;
window.closeProgressModal = closeProgressModal;
window.openModal = openModal;
fetchQuota();
fetchPlaylists();
//# sourceMappingURL=app.js.map