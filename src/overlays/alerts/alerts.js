const socket = io('/alerts');
const container = document.getElementById('alert-container');

let isPlaying = false;
const queue = [];
let appConfig = null;

// Fetch config initially
fetch('/api/config').then(r => r.json()).then(conf => {
    appConfig = conf;
});

// If backend broadcasts config changes to global we could listen, but reload is fine for overlays usually.

socket.on('alert', (data) => {
    queue.push(data);
    processQueue();
});

function formatAlertText(data, template) {
    if (template) {
        return template.replace(/{user}/g, data.user || 'Alguien')
                       .replace(/{amount}/g, data.amount || data.count || data.viewers || '')
                       .replace(/{tier}/g, data.tier || '')
                       .replace(/{message}/g, data.message || '');
    }

    // Fallbacks
    switch (data.type) {
        case 'superchat': return `<span>${data.user}</span> sent ${data.currency||''} ${data.amount}`;
        case 'subscription': return `<span>${data.user}</span> Subscribed! Tier ${data.tier}`;
        case 'bits': return `<span>${data.user}</span> cheered ${data.amount} Bits!`;
        case 'gift': return `<span>${data.user}</span> gifted ${data.count} ${data.giftName}s!`;
        case 'follow': return `<span>${data.user}</span> is now following!`;
        case 'raid': return `<span>${data.user}</span> is raiding with ${data.viewers} viewers!`;
        default: return `<span>${data.user}</span> triggered ${data.type}`;
    }
}

async function processQueue() {
    if (isPlaying || queue.length === 0) return;
    isPlaying = true;
    
    // Refresh config just in case
    try {
        const r = await fetch('/api/config');
        appConfig = await r.json();
    } catch(e){}

    const data = queue.shift();
    const conf = (appConfig && appConfig.alertsConfig && appConfig.alertsConfig[data.type]) || null;

    if (conf && conf.enabled === false) {
        isPlaying = false;
        processQueue();
        return;
    }

    const titleMsg = formatAlertText(data, conf ? conf.messageTemplate : null);
    
    const alertEl = document.createElement('div');
    alertEl.className = 'alert-box';
    alertEl.dataset.type = data.type;
    
    let mediaHtml = '';
    if (conf && conf.gifUrl) {
        mediaHtml = `<img src="${conf.gifUrl}" class="alert-gif" alt="Alert GIF" />`;
    }
    
    alertEl.innerHTML = `
        ${mediaHtml}
        <div class="alert-title">${titleMsg}</div>
        ${(!conf?.messageTemplate && data.message) ? `<div class="alert-message">${data.message}</div>` : ''}
    `;
    
    // Inject custom structural theme
    const themeStr = (conf && conf.theme) ? conf.theme : (appConfig?.chatTheme || 'cyberpurple');
    const wrapperEl = document.createElement('div');
    wrapperEl.className = `theme-${themeStr}`;
    wrapperEl.style.width = "100%";
    wrapperEl.style.display = "flex";
    wrapperEl.style.justifyContent = "center";
    
    wrapperEl.appendChild(alertEl);
    container.appendChild(wrapperEl);
    
    // Play SFX if defined
    if (conf && conf.audioUrl) {
        try {
            const audio = new Audio(conf.audioUrl);
            audio.volume = 0.5; // You could link this to master volume if desired
            audio.play().catch(e => console.warn('Audio play failed', e));
        } catch(e) {}
    }
    
    // Animate in
    setTimeout(() => {
        alertEl.classList.add('show');
    }, 50);
    
    // Hold for 4.5 seconds
    await new Promise(resolve => setTimeout(resolve, 4500));
    
    // Animate out
    alertEl.classList.replace('show', 'hide');
    
    // Remove from DOM
    setTimeout(() => {
        wrapperEl.remove();
        isPlaying = false;
        processQueue(); // Next
    }, 500);
}
