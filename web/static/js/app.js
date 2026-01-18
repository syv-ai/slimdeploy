// SlimDeploy JavaScript
// Minimal JS - most functionality is handled by HTMX

// Handle HTMX errors
document.body.addEventListener('htmx:responseError', function(evt) {
    console.error('HTMX error:', evt.detail.error);
    if (evt.detail.xhr.status === 401) {
        // Redirect to login on unauthorized
        window.location.href = '/login';
    }
});

// Handle HTMX before request (for loading states)
document.body.addEventListener('htmx:beforeRequest', function(evt) {
    const target = evt.detail.elt;
    if (target.tagName === 'BUTTON') {
        target.disabled = true;
    }
});

// Handle HTMX after request
document.body.addEventListener('htmx:afterRequest', function(evt) {
    const target = evt.detail.elt;
    if (target.tagName === 'BUTTON') {
        target.disabled = false;
    }
});

// Auto-refresh for deploying status
function setupStatusPolling() {
    const deployingCards = document.querySelectorAll('[data-status="deploying"]');
    deployingCards.forEach(card => {
        const projectId = card.id.replace('project-', '');
        setInterval(() => {
            htmx.trigger(card, 'refresh');
        }, 3000);
    });
}

// Initialize on DOM ready
document.addEventListener('DOMContentLoaded', function() {
    setupStatusPolling();
});

// Reinitialize after HTMX swaps
document.body.addEventListener('htmx:afterSwap', function() {
    setupStatusPolling();
});
