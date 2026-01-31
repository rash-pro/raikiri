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
    statusContainer.classList.add('visible'); // Show status indicators only in debug
}

let appConfig = {
    twitchChannels: [],
    youtubeId: '',
    kickChannel: '',
    tiktokChannel: ''
};

// Max messages to keep in the DOM
const MAX_MESSAGES = 100;

function appendMessage(data) {
    const msgDiv = document.createElement('div');
    msgDiv.classList.add('chat-message', `platform-${data.platform}`);
    if (data.id) msgDiv.dataset.id = data.id;

    // Check for Highlighting
    if (shouldHighlight(data)) {
        msgDiv.classList.add('highlight');
    }

    // Platform Badge
    const badge = document.createElement('span');
    badge.classList.add('platform-badge');
    if (data.platform === 'twitch') badge.innerText = 'TW';
    else if (data.platform === 'youtube') badge.innerText = 'YT';
    else if (data.platform === 'kick') badge.innerText = 'KI';
    else if (data.platform === 'tiktok') badge.innerText = 'TK';
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
    if (appConfig.kickChannel) appConfig.kickChannel = appConfig.kickChannel.toLowerCase();
    if (appConfig.tiktokChannel) appConfig.tiktokChannel = appConfig.tiktokChannel.toLowerCase();
});

function shouldHighlight(data) {
    const contentLower = data.content.toLowerCase();
    const userLower = data.user.toLowerCase();

    // Check Twitch Channels
    for (const chan of appConfig.twitchChannels) {
        if (contentLower.includes(chan)) return true;
    }

    // Check Kick Channel
    if (appConfig.kickChannel && contentLower.includes(appConfig.kickChannel)) return true;

    // Check TikTok Channel
    if (appConfig.tiktokChannel) {
        // TikTok usernames often start with @, normalize for comparison
        const tkChan = appConfig.tiktokChannel.startsWith('@') ? appConfig.tiktokChannel.substring(1) : appConfig.tiktokChannel;
        if (contentLower.includes(tkChan)) return true;
    }

    return false;
}
// Web Config Logic
const configBtn = document.getElementById('config-btn');
const configModal = document.getElementById('config-modal');
const configForm = document.getElementById('config-form');
const cancelBtn = document.getElementById('cancel-btn');
const saveBtn = document.getElementById('save-btn');
const modalBackdrop = document.querySelector('.modal-backdrop');

// Inputs
const inputTwitch = document.getElementById('twitchChannels');
const inputYouTube = document.getElementById('youtubeId');
const inputKick = document.getElementById('kickChannel');
const inputTikTok = document.getElementById('tiktokChannel');
const inputIgnored = document.getElementById('ignoredUsers');

function openModal() {
    configModal.classList.remove('hidden');
    // Fetch latest config to populate
    fetch('/api/config')
        .then(res => res.json())
        .then(config => {
            inputTwitch.value = config.twitchChannels ? config.twitchChannels.join(', ') : '';
            inputYouTube.value = config.youtube.liveId || config.youtube.channelId || '';
            inputKick.value = config.kickChannel || '';
            inputTikTok.value = config.tiktokChannel || '';
            inputIgnored.value = config.ignoredUsers ? config.ignoredUsers.join(', ') : '';
        })
        .catch(err => console.error('Error fetching config:', err));
}

function closeModal() {
    configModal.classList.add('hidden');
}

configBtn.addEventListener('click', openModal);
cancelBtn.addEventListener('click', closeModal);
modalBackdrop.addEventListener('click', closeModal);

configForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    const originalBtnText = saveBtn.innerText;
    saveBtn.innerText = 'Saving...';
    saveBtn.disabled = true;

    const newConfig = {
        twitchChannels: inputTwitch.value.split(',').map(s => s.trim()).filter(s => s),
        youtube: {
            // Simple heuristic: if it starts with UC, it's a channel, else liveId/videoId
            // Actually, let's just send it as liveId for now or let server decide.
            // But our server logic splits identifiers. Let's assume liveId if not clear.
            liveId: inputYouTube.value.trim().startsWith('UC') ? '' : inputYouTube.value.trim(),
            channelId: inputYouTube.value.trim().startsWith('UC') ? inputYouTube.value.trim() : ''
        },
        kickChannel: inputKick.value.trim(),
        tiktokChannel: inputTikTok.value.trim(),
        ignoredUsers: inputIgnored.value.split(',').map(s => s.trim().toLowerCase()).filter(s => s)
    };

    try {
        const res = await fetch('/api/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(newConfig)
        });
        const data = await res.json();

        if (data.success) {
            closeModal();
            // Optional: Show toast or feedback
        } else {
            alert('Error saving config: ' + data.message);
        }
    } catch (err) {
        console.error('Error saving config:', err);
        alert('Network error saving config');
    } finally {
        saveBtn.innerText = originalBtnText;
        saveBtn.disabled = false;
    }
});
