import { prepareRichInline, materializeRichInlineLineRange, walkRichInlineLineRanges } from "https://esm.sh/@chenglou/pretext@latest/rich-inline";

const socket = io('/chat');
const chatContainer = document.getElementById('chat-container');

// URL params configuration
const urlParams = new URLSearchParams(window.location.search);
const maxMessages = parseInt(urlParams.get('maxMessages')) || 50;
const allowedPlatformsCtx = urlParams.get('platforms');
const allowedPlatforms = allowedPlatformsCtx ? allowedPlatformsCtx.split(',') : ['twitch', 'youtube', 'kick', 'tiktok'];

const platformColors = {
    twitch: '#9146FF',
    youtube: '#FF0000',
    kick: '#53fc18',
    tiktok: '#ff0050'
};

const platformIcons = {
    twitch: '<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M11.571 4.714h1.715v5.143H11.57zm4.715 0H18v5.143h-1.714zM6 0L1.714 4.286v15.428h5.143V24l4.286-4.286h3.428L22.286 12V0zm14.571 11.143l-3.428 3.428h-3.429l-3 3v-3H6.857V1.714h13.714Z"/></svg>',
    youtube: '<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M23.498 6.186a3.016 3.016 0 0 0-2.122-2.136C19.505 3.545 12 3.545 12 3.545s-7.505 0-9.377.505A3.017 3.017 0 0 0 .502 6.186C0 8.07 0 12 0 12s0 3.93.502 5.814a3.016 3.016 0 0 0 2.122 2.136c1.871.505 9.376.505 9.376.505s7.505 0 9.377-.505a3.015 3.015 0 0 0 2.122-2.136C24 15.93 24 12 24 12s0-3.93-.502-5.814zM9.545 15.568V8.432L15.818 12l-6.273 3.568z"/></svg>',
    kick: '<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M11.517 9.873h-2.31v4.254h2.31v2.127h2.309v2.129h2.31v2.127H18.45V18.38h-2.31v-2.127h-2.31v-2.127h-2.31V9.872h2.31V7.745h2.31V5.618h2.31V3.491h-2.31v2.127h-2.31v2.127h-2.31v2.128zM3.468 3.49h2.31v17.02H3.468z"/></svg>',
    tiktok: '<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M19.59 6.69a4.83 4.83 0 0 1-3.77-4.25V2h-3.45v13.67a2.89 2.89 0 0 1-5.2 1.74 2.89 2.89 0 0 1 2.31-4.64 2.93 2.93 0 0 1 .88.13V9.4a6.84 6.84 0 0 0-1-.05A6.33 6.33 0 0 0 5 20.1a6.34 6.34 0 0 0 10.86-4.43v-7a8.16 8.16 0 0 0 4.77 1.52v-3.4a4.85 4.85 0 0 1-1-.1z"/></svg>'
};

let currentConfig = {
    chatTheme: 'glassmorphism',
    chatFontSize: 15,
    chatHideAfter: 30,
    chatAnimations: true
};

function applyConfig(config) {
    currentConfig = { ...currentConfig, ...config };
    
    // Apply Font Size
    document.documentElement.style.setProperty('--chat-font-size', `${currentConfig.chatFontSize}px`);
    
    // Apply Theme
    document.body.className = `theme-${currentConfig.chatTheme}`;
}

// Initial fetch
fetch('/api/config')
    .then(res => res.json())
    .then(applyConfig)
    .catch(console.error);

socket.on('config', (config) => {
    applyConfig(config);
});

function enforceMessageLimit() {
    while (chatContainer.children.length > maxMessages) {
        chatContainer.removeChild(chatContainer.firstChild);
    }
}

function createMessageElement(msg) {
    const el = document.createElement('div');
    const animClass = currentConfig.chatAnimations ? ' slide-in' : '';
    let baseClass = `message platform-${msg.platform}${animClass}`;
    if (msg.animationId) {
        baseClass += ` effect-${msg.animationId}`;
    }
    el.className = baseClass;
    el.dataset.id = msg.id;
    
    if (currentConfig.chatAnimations) {
        setTimeout(() => el.classList.remove('slide-in'), 300);
    }

    const color = msg.color || platformColors[msg.platform] || '#ffffff';
    
    // Badges implementation
    let badgesHtml = '';
    if (msg.badges && msg.badges.length > 0) {
        badgesHtml = '<span class="badges">';
        msg.badges.forEach(b => {
             // If we don't have a real image URL yet, use a styled text label
             if (b.url) {
                 badgesHtml += `<img src="${b.url}" class="badge-icon" alt="${b.type}">`;
             } else {
                 const badgeText = b.type === 'owner' ? 'HOST' : b.type.substring(0, 3).toUpperCase();
                 badgesHtml += `<span class="badge-text badge-${b.type}">${badgeText}</span>`;
             }
        });
        badgesHtml += '</span>';
    }

    const platformIconHtml = `<span class="platform-icon" style="color: ${platformColors[msg.platform]}">${platformIcons[msg.platform]}</span>`;

    // Structure
    el.innerHTML = `
        <div class="message-header">
            ${platformIconHtml}
            ${badgesHtml}
            <span class="username" style="color: ${color}">${msg.displayName}</span>
        </div>
        <div class="message-content"></div>
    `;

    const contentDiv = el.querySelector('.message-content');

    if (currentConfig.chatTheme === 'ffvi') {
        const temp = document.createElement('div');
        temp.innerHTML = msg.htmlContent;
        const nodes = Array.from(temp.childNodes);
        
        const items = [];
        const fontStr = `${currentConfig.chatFontSize}px "Press Start 2P", "Courier New", Courier, monospace`;
        
        nodes.forEach(node => {
            if (node.nodeType === Node.TEXT_NODE) {
                items.push({ text: node.nodeValue, font: fontStr });
            } else if (node.tagName === 'IMG') {
                // Emotes have .emote class. 
                // We use Zero-Width No-Break Space (\uFEFF) so pretext doesn't collapse consecutive emotes.
                items.push({ text: '\uFEFF', font: fontStr, break: 'never', extraWidth: currentConfig.chatFontSize * 2.5 });
            } else {
                items.push({ text: node.textContent || ' ', font: fontStr });
            }
        });

        if (items.length === 0) {
            items.push({ text: ' ', font: fontStr });
        }

        const padding = 100; // Total window to content padding adjusted 
        const baseWidth = window.innerWidth || document.documentElement.clientWidth;
        const maxWidth = Math.max(100, baseWidth - padding); 
        
        try {
            const prepared = prepareRichInline(items);
            let delayAcc = 0.12; // Esperar a que se termine de dibujar la ventana (0.12s)

            walkRichInlineLineRanges(prepared, maxWidth, range => {
                const line = materializeRichInlineLineRange(prepared, range);
                const lineDiv = document.createElement('div');
                lineDiv.className = 'chat-line';
                if (!currentConfig.chatAnimations) {
                    lineDiv.classList.add('no-anim');
                }

                let charCount = 0;
                line.fragments.forEach(frag => {
                    const originalNode = nodes[frag.itemIndex];
                    if (originalNode && originalNode.tagName === 'IMG') {
                        charCount += 4; // Emotes duration weight
                        lineDiv.appendChild(originalNode.cloneNode(true));
                    } else {
                        // text fragments can be sliced by pretext, so use frag.text!
                        charCount += frag.text.length;
                        lineDiv.appendChild(document.createTextNode(frag.text));
                    }
                });

                // Setting sequential CSS wipe effect - Fast 60fps typing (~16ms per char)
                const duration = Math.max(0.1, charCount * 0.016);
                if (currentConfig.chatAnimations) {
                    lineDiv.style.animationDuration = `${duration}s`;
                    lineDiv.style.animationDelay = `${delayAcc}s`;
                    lineDiv.style.animationTimingFunction = `steps(${Math.max(1, charCount)}, end)`;
                }
                delayAcc += duration;

                contentDiv.appendChild(lineDiv);
            });
        } catch(e) {
            console.error("Pretext error, falling back:", e);
            contentDiv.innerHTML = msg.htmlContent;
        }
    } else {
        contentDiv.innerHTML = msg.htmlContent;
    }

    // Auto-hide old messages logic
    if (currentConfig.chatHideAfter > 0) {
        setTimeout(() => {
            if (currentConfig.chatAnimations) {
                el.style.animation = 'fadeOut 0.5s ease-in forwards';
                setTimeout(() => { if (el.parentNode) el.parentNode.removeChild(el); }, 500);
            } else {
                if (el.parentNode) el.parentNode.removeChild(el);
            }
        }, currentConfig.chatHideAfter * 1000);
    }

    return el;
}

socket.on('message', async (msg) => {
    if (!allowedPlatforms.includes(msg.platform)) return;
    
    if (currentConfig.chatTheme === 'ffvi') {
        const fontStr = `${currentConfig.chatFontSize}px "Press Start 2P"`;
        if (document.fonts) {
            try { await document.fonts.load(fontStr); } catch(e) {}
        }
    }
    
    const msgEl = createMessageElement(msg);
    chatContainer.appendChild(msgEl);
    
    // Auto-scroll
    window.scrollTo({
        top: document.body.scrollHeight,
        behavior: 'smooth'
    });
    
    enforceMessageLimit();
});

// For testing connection
socket.on('connect', () => {
    console.log('Connected to Chat Overlay namespace');
});
