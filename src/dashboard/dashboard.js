document.addEventListener('DOMContentLoaded', () => {
    // Nav behavior
    const navItems = document.querySelectorAll('.nav-item');
    const sections = document.querySelectorAll('.view-section');
    const pageTitle = document.getElementById('page-title');
    const pageSubtitle = document.getElementById('page-subtitle');
    
    const sectionmeta = {
        '#platforms': { title: 'Platforms Setup', sub: 'Connect your streaming accounts for real-time events.' },
        '#tts': { title: 'Audio & TTS', sub: 'Configure Edge TTS integration and audio routing.' },
        '#overlays': { title: 'Overlays', sub: 'Browser sources to add directly to OBS.' }
    };

    navItems.forEach(item => {
        item.addEventListener('click', (e) => {
            e.preventDefault();
            const targetHash = item.getAttribute('href');
            
            navItems.forEach(n => n.classList.remove('active'));
            item.classList.add('active');
            
            sections.forEach(s => s.classList.remove('active'));
            document.querySelector(targetHash.replace('#', '#section-')).classList.add('active');
            
            if (sectionmeta[targetHash]) {
                pageTitle.innerText = sectionmeta[targetHash].title;
                pageSubtitle.innerText = sectionmeta[targetHash].sub;
            }
        });
    });

    let currentConfig = {};

    // Build payload to save config
    function saveConfig() {
        const payload = {};
        
        ['twitchClientId', 'youtubeChannelId', 'kickUsername', 'tiktokUsername', 
         'ttsEnabled', 'ttsVoice', 'ttsMinBits', 'audioMode', 'ttsSubTier', 'audioVolume',
         'ttsRewardEnabled', 'ttsRewardName', 'ttsCmdEnabled', 'ttsCmdPrefix', 
         'ttsCmdMod', 'ttsCmdSub', 'ttsCmdVip', 'ttsCmdHost',
         'chatTheme', 'chatFontSize', 'chatHideAfter', 'chatAnimations'].forEach(key => {
            const el = document.getElementById(`config-${key}`);
            if (el) {
                if (el.type === 'checkbox') payload[key] = el.checked;
                else if (el.type === 'number' || el.type === 'range') payload[key] = Number(el.value);
                else {
                    if (key === 'chatAnimations') payload[key] = el.value === 'true';
                    else payload[key] = el.value;
                }
            }
        });

        // Save current alert type UI to cache before saving
        const aType = document.getElementById('config-alertType')?.value;
        if (aType && currentConfig.alertsConfig) {
            currentConfig.alertsConfig[aType] = {
                enabled: document.getElementById('config-alertEnabled').checked,
                theme: document.getElementById('config-alertTheme').value,
                voice: document.getElementById('config-alertVoice').value,
                audioUrl: document.getElementById('config-alertAudioUrl').value,
                gifUrl: document.getElementById('config-alertGifUrl').value,
                messageTemplate: document.getElementById('config-alertMessageTemplate').value,
            };
        }
        
        payload.alertsConfig = currentConfig.alertsConfig || {};

        return payload;
    }

    // Load Config
    async function loadConfig() {
        try {
            const res = await fetch('/api/config');
            const data = await res.json();
            currentConfig = data;
            
            // Populate inputs
            Object.keys(data).forEach(key => {
                if (key === 'alertsConfig') return; // Handled separately
                const el = document.getElementById(`config-${key}`);
                if (el) {
                    if (el.type === 'checkbox') {
                        el.checked = data[key];
                    } else {
                        if (key === 'chatAnimations') el.value = data[key] ? 'true' : 'false';
                        else el.value = data[key];
                    }
                    if (key === 'audioVolume') {
                        document.getElementById('volumeVal').innerText = `${data[key]}%`;
                    }
                }
            });
            
            // Initialize alerts UI mapping
            if (window.loadAlertConfigUI) window.loadAlertConfigUI();
            
        } catch (err) {
            console.error('Failed to load config:', err);
        }
    }
    loadConfig();

    // Volume slider sync
    const volSlider = document.getElementById('config-audioVolume');
    if(volSlider) {
        volSlider.addEventListener('input', (e) => {
            document.getElementById('volumeVal').innerText = `${e.target.value}%`;
        });
    }

    // Save Config
    document.getElementById('saveBtn').addEventListener('click', async () => {
        const payload = saveConfig();

        try {
            const res = await fetch('/api/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
            if (res.ok) {
                showToast('Configuration Saved!');
            }
        } catch (err) {
            alert('Failed to save config');
        }
    });

    // TTS Test Button
    const ttsBtn = document.getElementById('testTtsBtn');
    if (ttsBtn) {
        ttsBtn.addEventListener('click', async () => {
             try {
                // Good UX: we might want to tell them to configure and save first if they haven't
                const res = await fetch('/api/tts/test', { method: 'POST' });
                if (res.ok) {
                    showToast('Playing test audio. Ensure your Audio Hub is open!');
                }
             } catch (err) {
                 alert('Error triggering TTS');
             }
        });
    }

    // Chat Test Button
    const chatBtn = document.getElementById('testChatBtn');
    if (chatBtn) {
        chatBtn.addEventListener('click', async () => {
             try {
                const res = await fetch('/api/chat/test', { method: 'POST' });
                if (res.ok) {
                    showToast('Chat message sent! Check your Chat Overlay tab or OBS.');
                }
             } catch (err) {
                 alert('Error triggering chat');
             }
        });
    }

    // Twitch Device Auth Flow
    document.getElementById('authTwitchBtn').addEventListener('click', async () => {
        const clientId = document.getElementById('config-twitchClientId').value;
        if (!clientId) {
            alert("Please save your Client ID first!");
            return;
        }

        const btn = document.getElementById('authTwitchBtn');
        btn.innerText = "Requesting...";
        btn.disabled = true;

        try {
            const res = await fetch('/api/auth/twitch/device-code', { method: 'POST' });
            const data = await res.json();
            
            if (!res.ok) {
               throw new Error(data.error || "Failed to start auth");
            }
            
            document.getElementById('twitchUserCode').innerText = data.userCode;
            document.getElementById('twitchVerifyUri').href = data.verificationUri;
            document.getElementById('twitchAuthDisplay').style.display = 'block';
            
            btn.innerText = "Waiting for authorization...";
            
            // In a real app we might get a WebSocket event when it finishes,
            // but for simplicity we rely on the backend to tell us, or we just notify
            // actually since we didn't hook up a WS for status, we'll just say:
            showToast("Follow the link and enter the code. The system will auto-connect.", 6000);
            
        } catch (err) {
            alert(err.message);
            btn.innerText = "Authenticate Device";
            btn.disabled = false;
        }
    });

    function showToast(msg, duration = 3000) {
        const toast = document.getElementById('toast');
        toast.innerText = msg;
        toast.classList.add('show');
        setTimeout(() => toast.classList.remove('show'), duration);
    }
    
    // Global scopes for inline HTML handlers
    let prevAlert = 'follow';
    window.loadAlertConfigUI = function() {
        if (!currentConfig || !currentConfig.alertsConfig) return;
        
        // Save previous before switching
        if (prevAlert) {
            currentConfig.alertsConfig[prevAlert] = {
                enabled: document.getElementById('config-alertEnabled').checked,
                theme: document.getElementById('config-alertTheme').value,
                voice: document.getElementById('config-alertVoice').value,
                audioUrl: document.getElementById('config-alertAudioUrl').value,
                gifUrl: document.getElementById('config-alertGifUrl').value,
                messageTemplate: document.getElementById('config-alertMessageTemplate').value,
            };
        }
        
        const aType = document.getElementById('config-alertType').value;
        prevAlert = aType;
        const aConf = currentConfig.alertsConfig[aType] || { enabled: true, theme: 'cyberpurple', voice: '', audioUrl: '', gifUrl: '', messageTemplate: '' };
        
        document.getElementById('config-alertEnabled').checked = aConf.enabled !== false;
        document.getElementById('config-alertTheme').value = aConf.theme || 'cyberpurple';
        document.getElementById('config-alertVoice').value = aConf.voice || '';
        document.getElementById('config-alertAudioUrl').value = aConf.audioUrl || '';
        document.getElementById('config-alertGifUrl').value = aConf.gifUrl || '';
        document.getElementById('config-alertMessageTemplate').value = aConf.messageTemplate || '';
    };

    window.testCurrentAlert = function() {
        const type = document.getElementById('config-alertType').value;
        fetch('/api/alerts/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ type })
        }).then(res => res.json())
          .catch(console.error);
    };
});
