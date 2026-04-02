// BrewOps Dashboard — Real-time HTCPCP/1.0 Observability
// "A watched pot never boils" — but we watch them anyway.

(function() {
    'use strict';

    const MAX_EVENTS = 100;
    const host = window.location.origin;

    // Set the command host in the "Try It" section
    document.querySelectorAll('.cmd-host').forEach(el => {
        el.textContent = host;
    });

    // Click to copy commands
    document.querySelectorAll('.try-cmd code').forEach(el => {
        el.addEventListener('click', () => {
            navigator.clipboard.writeText(el.textContent).then(() => {
                el.classList.add('copied');
                setTimeout(() => el.classList.remove('copied'), 1000);
            });
        });
    });

    // =====================================================
    // SSE Connection
    // =====================================================
    let eventsLog = document.getElementById('events-log');
    let eventCount = 0;

    function connectSSE() {
        const evtSource = new EventSource(host + '/events');

        evtSource.onmessage = function(e) {
            try {
                const event = JSON.parse(e.data);
                addEvent(event);
            } catch (err) {
                console.error('Failed to parse event:', err);
            }
        };

        evtSource.addEventListener('stats', function(e) {
            try {
                const stats = JSON.parse(e.data);
                updateStats(stats);
            } catch (err) {
                console.error('Failed to parse stats:', err);
            }
        });

        evtSource.addEventListener('pots', function(e) {
            try {
                const pots = JSON.parse(e.data);
                updatePots(pots);
            } catch (err) {
                console.error('Failed to parse pots:', err);
            }
        });

        evtSource.onerror = function() {
            console.log('SSE connection lost. Reconnecting in 3s...');
            evtSource.close();
            setTimeout(connectSSE, 3000);
        };
    }

    // =====================================================
    // Event Log
    // =====================================================
    function addEvent(event) {
        // Remove the "waiting" message if present
        const empty = eventsLog.querySelector('.event-empty');
        if (empty) empty.remove();

        const row = document.createElement('div');
        row.className = 'event-row severity-' + event.severity;

        // Flash animation for 418s
        if (event.status === 418) {
            row.classList.add('flash');
        }

        row.innerHTML =
            '<span class="event-time">' + escapeHtml(event.timestamp) + '</span>' +
            '<span class="event-severity ' + escapeHtml(event.severity) + '">' + escapeHtml(event.severity) + '</span>' +
            '<span class="event-ip">' + escapeHtml(event.ip) + '</span>' +
            '<span class="event-method">' + escapeHtml(event.method) + ' ' + escapeHtml(event.path) + '</span>' +
            '<span class="event-message">' + escapeHtml(event.message) + '</span>';

        // Insert at top (newest first)
        eventsLog.insertBefore(row, eventsLog.firstChild);

        // Limit total events
        eventCount++;
        while (eventsLog.children.length > MAX_EVENTS) {
            eventsLog.removeChild(eventsLog.lastChild);
        }
    }

    // =====================================================
    // Stats Updates
    // =====================================================
    function updateStats(stats) {
        animateValue('stat-brews', stats.total_brews);
        animateValue('stat-418s', stats.total_418s);
        animateValue('stat-brewers', stats.unique_brewers);
        animateValue('stat-docs', stats.docs_attacks);

        var rateEl = document.getElementById('stat-rate-val');
        if (rateEl) rateEl.textContent = stats.rate_418_percent.toFixed(1);

        var caffeineEl = document.getElementById('stat-caffeine');
        if (caffeineEl) caffeineEl.textContent = formatNumber(stats.caffeine_dispensed_mg);

        var addEl = document.getElementById('stat-addition');
        if (addEl) addEl.textContent = stats.most_popular_addition || 'None yet';

        var uptimeEl = document.getElementById('sla-uptime');
        if (uptimeEl) uptimeEl.textContent = stats.brew_uptime_percent.toFixed(2) + '%';

        var spillsEl = document.getElementById('sla-spills');
        if (spillsEl) spillsEl.textContent = stats.spills_this_quarter;

        var slaDocsEl = document.getElementById('sla-docs');
        if (slaDocsEl) slaDocsEl.textContent = stats.docs_attacks;
    }

    function animateValue(elId, newValue) {
        const el = document.getElementById(elId);
        if (!el) return;
        const current = parseInt(el.textContent) || 0;
        if (current === newValue) return;

        // Simple animation: jump to new value with a brief highlight
        el.textContent = formatNumber(newValue);
        el.style.transition = 'color 0.3s';
        el.style.color = '#ff4e1f';
        setTimeout(function() {
            el.style.color = '';
        }, 400);
    }

    // =====================================================
    // Pot Status Cards
    // =====================================================
    function updatePots(pots) {
        const grid = document.getElementById('pots-grid');
        if (!grid) return;

        // Build cards
        grid.innerHTML = pots.map(pot => {
            const isTeapot = pot.type === 'teapot';
            const isBrewing = pot.state === 'brewing' || pot.state === 'grinding' || pot.state === 'pouring';
            const icon = isTeapot ? '\uD83E\uDED6' : '\u2615';
            const typeClass = isTeapot ? 'pot-teapot' : (isBrewing ? 'pot-brewing' : '');
            const typeBadgeClass = isTeapot ? 'pot-type-teapot' : 'pot-type-coffee';

            const tempPercent = Math.min(100, Math.max(0, (pot.temperature_celsius / 100) * 100));
            let tempClass = 'temp-cold';
            if (pot.temperature_celsius > 60) tempClass = 'temp-warm';
            if (pot.temperature_celsius > 85) tempClass = 'temp-hot';

            const stateEmoji = getStateEmoji(pot.state);

            return '<div class="pot-card ' + typeClass + '">' +
                '<div class="pot-header">' +
                    '<span class="pot-name">' + icon + ' pot-' + pot.id + '</span>' +
                    '<span class="pot-type-badge ' + typeBadgeClass + '">' + escapeHtml(pot.type) + '</span>' +
                '</div>' +
                '<div class="pot-details">' +
                    '<span class="pot-detail-label">State</span>' +
                    '<span class="pot-detail-value">' + stateEmoji + ' ' + escapeHtml(pot.state) + '</span>' +
                    '<span class="pot-detail-label">Temperature</span>' +
                    '<span class="pot-detail-value">' + pot.temperature_celsius + '\u00b0C</span>' +
                    '<span class="pot-detail-label">Condition</span>' +
                    '<span class="pot-detail-value">' + escapeHtml(pot.temperature_label) + '</span>' +
                    '<span class="pot-detail-label">Fill Level</span>' +
                    '<span class="pot-detail-value">' + pot.fill_level_percent + '%</span>' +
                    (pot.beverage ? '<span class="pot-detail-label">Beverage</span><span class="pot-detail-value">' + escapeHtml(pot.beverage) + '</span>' : '') +
                    (pot.brew_elapsed ? '<span class="pot-detail-label">Brew Time</span><span class="pot-detail-value">' + escapeHtml(pot.brew_elapsed) + '</span>' : '') +
                    '<span class="pot-detail-label">Safe</span>' +
                    '<span class="pot-detail-value">' + escapeHtml(pot.safe) + '</span>' +
                '</div>' +
                '<div class="pot-temp-bar">' +
                    '<div class="pot-temp-fill ' + tempClass + '" style="width: ' + tempPercent + '%"></div>' +
                '</div>' +
            '</div>';
        }).join('');

        // Update sidebar fleet counters
        var potCountEl = document.getElementById('sidebar-pot-count');
        if (potCountEl) potCountEl.textContent = pots.length;

        var teapotCount = 0;
        for (var i = 0; i < pots.length; i++) {
            if (pots[i].type === 'teapot') teapotCount++;
        }
        var teapotCountEl = document.getElementById('sidebar-teapot-count');
        if (teapotCountEl) teapotCountEl.textContent = teapotCount;
    }

    function getStateEmoji(state) {
        var emojis = {
            'idle': '\u23F8\uFE0F',
            'grinding': '\u2699\uFE0F',
            'brewing': '\uD83D\uDD25',
            'pouring': '\uD83E\uDED7',
            'ready': '\u2705',
            'cooling': '\u2744\uFE0F'
        };
        return emojis[state] || '\u2753';
    }

    // =====================================================
    // Helpers
    // =====================================================
    function escapeHtml(str) {
        if (typeof str !== 'string') return String(str);
        return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    function formatNumber(n) {
        if (typeof n !== 'number') return String(n);
        if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
        if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
        return Math.round(n).toLocaleString();
    }

    // =====================================================
    // Initialize
    // =====================================================
    connectSSE();

    // Fetch initial pot status
    fetch(host + '/status')
        .then(r => r.json())
        .then(data => {
            if (data.pots) updatePots(data.pots);
            if (data.stats) updateStats(data.stats);
        })
        .catch(err => console.error('Failed to fetch initial status:', err));

})();
