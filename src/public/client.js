const socket = io();
const chatContainer = document.getElementById('chat-container');
const statusContainer = document.getElementById('status-container');
const debugContainer = document.createElement('div'); // Debug panel

// Check for debug mode
const urlParams = new URLSearchParams(window.location.search);
const isDebug = urlParams.get('debug') === 'true';

if (isDebug) {
    debugContainer.id = 'debug-panel';
    document.body.appendChild(debugContainer);
}

let appConfig = {
    twitchChannels: [],
    youtubeId: ''
};

// Max messages to keep in the DOM
const MAX_MESSAGES = 100;

function appendMessage(data) {
    const msgDiv = document.createElement('div');
    msgDiv.classList.add('chat-message', `platform-${data.platform}`);
    if (data.id) msgDiv.dataset.id = data.id;

    // Check for Highlighting
    if (shouldHighlight(data.content)) {
        msgDiv.classList.add('highlight');
    }

    // Platform Badge
    const badge = document.createElement('span');
    badge.classList.add('platform-badge');
    badge.innerText = data.platform === 'twitch' ? 'TW' : 'YT';
    msgDiv.appendChild(badge);

    badge.innerText = data.platform === 'twitch' ? 'TW' : 'YT';
    msgDiv.appendChild(badge);

    // User Badges
    if (data.badges && Array.isArray(data.badges)) {
        data.badges.forEach(type => {
            const badgeSpan = document.createElement('span');
            badgeSpan.classList.add('user-badge', `badge-${type}`);

            // Icons
            switch (type) {
                case 'owner': badgeSpan.textContent = '👑'; break;
                case 'moderator': badgeSpan.textContent = '🛡️'; break;
                case 'vip': badgeSpan.textContent = '💎'; break;
                case 'subscriber': badgeSpan.textContent = '⭐'; break;
                default: badgeSpan.textContent = '';
            }

            if (badgeSpan.textContent) msgDiv.appendChild(badgeSpan);
        });
    }

    // Author
    const authorSpan = document.createElement('span');
    authorSpan.classList.add('author');
    authorSpan.innerText = data.user;
    if (data.color) {
        authorSpan.style.color = data.color;
    }
    msgDiv.appendChild(authorSpan);

    // Content
    const contentSpan = document.createElement('span');
    contentSpan.classList.add('content');

    if (data.isHtml) {
        contentSpan.innerHTML = data.content; // Server has already sanitized the text parts
    } else {
        contentSpan.textContent = data.content;
    }

    msgDiv.appendChild(contentSpan);

    chatContainer.appendChild(msgDiv);

    // Auto-scroll to bottom
    window.scrollTo(0, document.body.scrollHeight);

    // Prune old messages
    while (chatContainer.children.length > MAX_MESSAGES) {
        chatContainer.removeChild(chatContainer.firstChild);
    }
}

socket.on('chat_message', (data) => {
    appendMessage(data);
});

// Status Handling
const statuses = {}; // { twitch: element, youtube: element }

function updateStatus(platform, state, message) {
    if (!statuses[platform]) {
        const div = document.createElement('div');
        div.classList.add('status-item');

        const dot = document.createElement('div');
        dot.classList.add('status-indicator');

        const text = document.createElement('span');
        text.innerText = platform.toUpperCase();

        div.appendChild(dot);
        div.appendChild(text);
        statusContainer.appendChild(div);
        statuses[platform] = div;
    }

    const el = statuses[platform];
    el.className = `status-item status-${state}`;

    // Optional: Hide after success? 
    // For now, let's keep it visible so user knows if it drops.
    if (state === 'error' && message) {
        el.title = message;
    }
}

socket.on('status', (data) => {
    updateStatus(data.platform, data.state, data.message);
});

socket.on('debug_log', (data) => {
    if (!isDebug) return;

    const logDiv = document.createElement('div');
    logDiv.classList.add('debug-log', `log-${data.level}`);
    logDiv.innerText = `[${data.platform}] ${data.message} ${data.details ? JSON.stringify(data.details) : ''}`;
    debugContainer.prepend(logDiv);

    // Prune logs
    if (debugContainer.children.length > 50) {
        debugContainer.removeChild(debugContainer.lastChild);
    }
});

socket.on('connect', () => {
    console.log('Connected to chat server');
});

socket.on('config', (config) => {
    appConfig = config;
    // Normalize config
    appConfig.twitchChannels = appConfig.twitchChannels.map(c => c.toLowerCase());
});

function shouldHighlight(content) {
    const lower = content.toLowerCase();

    // Check Twitch Channels
    for (const chan of appConfig.twitchChannels) {
        if (lower.includes(chan)) return true;
    }

    // Hardcoded "Streamer" keywords could go here, 
    // but verifying against channel name is safest default.
    return false;
}
