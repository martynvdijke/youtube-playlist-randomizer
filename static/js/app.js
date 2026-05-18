"use strict";
function showModal() {
    const modal = document.getElementById("modal");
    if (modal)
        modal.classList.remove("hidden");
}
function closeModal() {
    const modal = document.getElementById("modal");
    if (modal) {
        modal.classList.add("hidden");
        modal.innerHTML = "";
    }
}
function showProgressModal() {
    closeModal();
    const modal = document.getElementById("progress-modal");
    if (modal)
        modal.classList.remove("hidden");
}
function closeProgressModal() {
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
//# sourceMappingURL=app.js.map