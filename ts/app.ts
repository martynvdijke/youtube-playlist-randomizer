function showModal(): void {
  const modal = document.getElementById("modal");
  if (modal) modal.classList.remove("hidden");
}

function closeModal(): void {
  const modal = document.getElementById("modal");
  if (modal) {
    modal.classList.add("hidden");
    modal.innerHTML = "";
  }
}

function showProgressModal(): void {
  closeModal();
  const modal = document.getElementById("progress-modal");
  if (modal) modal.classList.remove("hidden");
}

function closeProgressModal(): void {
  const modal = document.getElementById("progress-modal");
  if (modal) {
    modal.classList.add("hidden");
    modal.innerHTML = "";
  }
}

window.showModal = showModal;
window.closeModal = closeModal;
window.showProgressModal = showProgressModal;
window.closeProgressModal = closeProgressModal;
