document.addEventListener('DOMContentLoaded', () => {
    // Nav behavior
    const navItems = document.querySelectorAll('.nav-item');
    const sections = document.querySelectorAll('.view-section');
    const pageTitle = document.getElementById('page-title');
    const pageSubtitle = document.getElementById('page-subtitle');
    
    const sectionmeta = {
        '#platforms': { title: 'Platforms Setup', sub: 'Connect your streaming accounts for real-time events.' },
        '#tts': { title: 'Audio & TTS', sub: 'Configure Edge TTS integration and audio routing.' },
        '#overlays': { title: 'Overlays', sub: 'Browser sources to add directly to OBS.' },
        '#widgets': { title: 'Widgets', sub: 'Configurable browser sources for stream scenes.' }
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

    document.querySelectorAll('[id^="config-"]').forEach(el => {
        if (!el.name) el.name = el.id.replace('config-', '');
    });

    // Build payload to save config
    function saveConfig() {
        const payload = {};
        
        ['twitchClientId', 'twitchChannel', 'youtubeChannelId',
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
        payload.widgetsConfig = readWidgetsConfig();

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
                if (key === 'alertsConfig' || key === 'widgetsConfig') return; // Handled separately
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
            loadWidgetsConfigUI();
            
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

    document.body.addEventListener('htmx:configRequest', (event) => {
        if (event.target.id !== 'config-form') return;
        const payload = saveConfig();
        event.detail.parameters = {};
        Object.entries(payload).forEach(([key, value]) => {
            event.detail.parameters[key] = typeof value === 'object' ? JSON.stringify(value) : String(value);
        });
    });

    document.body.addEventListener('htmx:afterRequest', (event) => {
        if (event.target.id !== 'config-form') return;
        if (event.detail.successful) {
            showToast(document.getElementById('toast').innerText || 'Configuration Saved!');
            loadConfig();
        } else {
            alert(event.detail.xhr.responseText || 'Failed to save config');
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

    const resetSupportGoalBtn = document.getElementById('resetSupportGoalBtn');
    if (resetSupportGoalBtn) {
        resetSupportGoalBtn.addEventListener('click', async () => {
            try {
                const res = await fetch('/api/widgets/support-goal/reset', { method: 'POST' });
                if (!res.ok) throw new Error(await res.text());
                showToast('Support goal reset.');
            } catch (err) {
                alert(err.message || 'Error resetting support goal');
            }
        });
    }

    const testSupportGoalBtn = document.getElementById('testSupportGoalBtn');
    if (testSupportGoalBtn) {
        testSupportGoalBtn.addEventListener('click', () => testWidget('support', 'Support goal test sent.'));
    }

    const testRecentEventsBtn = document.getElementById('testRecentEventsBtn');
    if (testRecentEventsBtn) {
        testRecentEventsBtn.addEventListener('click', () => testWidget('recent', 'Recent event test sent.'));
    }

    const addCustomWidgetBtn = document.getElementById('addCustomWidgetBtn');
    if (addCustomWidgetBtn) {
        addCustomWidgetBtn.addEventListener('click', () => {
            commitCurrentCustomWidget();
            const widgets = ensureCustomWidgets();
            const nextIndex = widgets.length + 1;
            widgets.push(defaultCustomWidget(nextIndex));
            currentCustomWidgetIndex = widgets.length - 1;
            loadCustomWidgetPicker();
            loadCurrentCustomWidgetUI();
        });
    }

    const deleteCustomWidgetBtn = document.getElementById('deleteCustomWidgetBtn');
    if (deleteCustomWidgetBtn) {
        deleteCustomWidgetBtn.addEventListener('click', () => {
            const widgets = ensureCustomWidgets();
            if (!widgets.length) return;
            widgets.splice(currentCustomWidgetIndex, 1);
            currentCustomWidgetIndex = Math.max(0, currentCustomWidgetIndex - 1);
            loadCustomWidgetPicker();
            loadCurrentCustomWidgetUI();
        });
    }

    const customSelect = document.getElementById('widget-custom-select');
    if (customSelect) {
        customSelect.addEventListener('change', () => {
            const nextIndex = Number(customSelect.value || 0);
            commitCurrentCustomWidget();
            currentCustomWidgetIndex = nextIndex;
            loadCustomWidgetPicker();
            loadCurrentCustomWidgetUI();
        });
    }

    const copyCustomWidgetBtn = document.getElementById('copyCustomWidgetBtn');
    if (copyCustomWidgetBtn) {
        copyCustomWidgetBtn.addEventListener('click', () => {
            navigator.clipboard.writeText(customWidgetURL());
            showToast('Custom widget URL copied.');
        });
    }

    const testCustomWidgetBtn = document.getElementById('testCustomWidgetBtn');
    if (testCustomWidgetBtn) {
        testCustomWidgetBtn.addEventListener('click', () => {
            commitCurrentCustomWidget();
            const widget = ensureCustomWidgets()[currentCustomWidgetIndex];
            testWidget('custom', 'Custom widget test sent.', widget?.id || '');
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

    async function testWidget(kind, message, widgetId = '') {
        try {
            const res = await fetch('/api/widgets/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ kind, widgetId })
            });
            if (!res.ok) throw new Error(await res.text());
            showToast(message);
        } catch (err) {
            alert(err.message || 'Error triggering widget test');
        }
    }

    function defaultWidgetsConfig() {
        return {
            supportGoal: {
                enabled: true, title: 'Support Goal', targetAmount: 100, currency: 'USD',
                appearance: defaultAppearance(720, true)
            },
            recentEvents: {
                enabled: true,
                limit: 8,
                types: ['superchat', 'supersticker', 'membership', 'subscription', 'bits', 'raid', 'gift', 'follow'],
                appearance: defaultAppearance(520, true)
            },
            custom: []
        };
    }

    function widgetsConfig() {
        const defaults = defaultWidgetsConfig();
        const config = { ...defaults, ...(currentConfig.widgetsConfig || {}) };
        config.supportGoal = { ...defaults.supportGoal, ...(config.supportGoal || {}) };
        config.supportGoal.appearance = { ...defaults.supportGoal.appearance, ...(config.supportGoal.appearance || {}) };
        config.recentEvents = { ...defaults.recentEvents, ...(config.recentEvents || {}) };
        config.recentEvents.appearance = { ...defaults.recentEvents.appearance, ...(config.recentEvents.appearance || {}) };
        config.custom = Array.isArray(config.custom) ? config.custom : [];
        return config;
    }

    function loadWidgetsConfigUI() {
        const cfg = widgetsConfig();
        const support = { ...defaultWidgetsConfig().supportGoal, ...(cfg.supportGoal || {}) };
        const recent = { ...defaultWidgetsConfig().recentEvents, ...(cfg.recentEvents || {}) };

        const setChecked = (id, value) => {
            const el = document.getElementById(id);
            if (el) el.checked = value !== false;
        };
        const setValue = (id, value) => {
            const el = document.getElementById(id);
            if (el) el.value = value ?? '';
        };

        setChecked('widget-supportGoal-enabled', support.enabled);
        setValue('widget-supportGoal-title', support.title);
        setValue('widget-supportGoal-currency', support.currency);
        setValue('widget-supportGoal-targetAmount', support.targetAmount);
        loadAppearanceUI('widget-supportGoal', support.appearance, defaultAppearance(720, true));
        setChecked('widget-recentEvents-enabled', recent.enabled);
        setValue('widget-recentEvents-limit', recent.limit);
        setValue('widget-recentEvents-types', (recent.types || []).join(','));
        loadAppearanceUI('widget-recentEvents', recent.appearance, defaultAppearance(520, true));
        loadCustomWidgetPicker();
        loadCurrentCustomWidgetUI();
    }

    function readWidgetsConfig() {
        commitCurrentCustomWidget();
        const current = widgetsConfig();
        const support = current.supportGoal || {};
        const recent = current.recentEvents || {};
        const supportTarget = Number(document.getElementById('widget-supportGoal-targetAmount')?.value || support.targetAmount || 100);
        const recentLimit = Number(document.getElementById('widget-recentEvents-limit')?.value || recent.limit || 8);
        const recentTypes = (document.getElementById('widget-recentEvents-types')?.value || '')
            .split(',')
            .map(type => type.trim())
            .filter(Boolean);
        currentConfig.widgetsConfig = {
            supportGoal: {
                enabled: document.getElementById('widget-supportGoal-enabled')?.checked !== false,
                title: document.getElementById('widget-supportGoal-title')?.value || support.title || 'Support Goal',
                currency: document.getElementById('widget-supportGoal-currency')?.value || support.currency || 'USD',
                targetAmount: Math.max(1, supportTarget),
                appearance: readAppearanceUI('widget-supportGoal', support.appearance, defaultAppearance(720, true))
            },
            recentEvents: {
                enabled: document.getElementById('widget-recentEvents-enabled')?.checked !== false,
                limit: Math.min(20, Math.max(1, recentLimit)),
                types: recentTypes.length ? recentTypes : recent.types,
                appearance: readAppearanceUI('widget-recentEvents', recent.appearance, defaultAppearance(520, true))
            },
            custom: ensureCustomWidgets()
        };
        return currentConfig.widgetsConfig;
    }

    function defaultAppearance(width, showIcons) {
        return {
            theme: 'glass',
            accentColor: '#10b981',
            fontFamily: 'Inter, system-ui, sans-serif',
            backgroundOpacity: 78,
            borderRadius: 8,
            width,
            showIcons
        };
    }

    function loadAppearanceUI(prefix, appearance, defaults) {
        const value = { ...defaults, ...(appearance || {}) };
        setInput(`${prefix}-theme`, value.theme);
        setInput(`${prefix}-accentColor`, value.accentColor);
        setInput(`${prefix}-fontFamily`, value.fontFamily);
        setInput(`${prefix}-backgroundOpacity`, value.backgroundOpacity);
        setInput(`${prefix}-borderRadius`, value.borderRadius);
        setInput(`${prefix}-width`, value.width);
        const icons = document.getElementById(`${prefix}-showIcons`);
        if (icons) icons.checked = value.showIcons !== false;
    }

    function readAppearanceUI(prefix, appearance, defaults) {
        const value = { ...defaults, ...(appearance || {}) };
        return {
            theme: getInput(`${prefix}-theme`, value.theme),
            accentColor: getInput(`${prefix}-accentColor`, value.accentColor),
            fontFamily: getInput(`${prefix}-fontFamily`, value.fontFamily),
            backgroundOpacity: clamp(Number(getInput(`${prefix}-backgroundOpacity`, value.backgroundOpacity)), 0, 100),
            borderRadius: clamp(Number(getInput(`${prefix}-borderRadius`, value.borderRadius)), 0, 40),
            width: clamp(Number(getInput(`${prefix}-width`, value.width)), 240, 1920),
            showIcons: document.getElementById(`${prefix}-showIcons`)?.checked ?? value.showIcons
        };
    }

    function setInput(id, value) {
        const el = document.getElementById(id);
        if (el) el.value = value ?? '';
    }

    function getInput(id, fallback) {
        const el = document.getElementById(id);
        return el && el.value !== '' ? el.value : fallback;
    }

    function clamp(value, min, max) {
        if (!Number.isFinite(value)) return min;
        return Math.min(max, Math.max(min, value));
    }

    let currentCustomWidgetIndex = 0;

    function ensureCustomWidgets() {
        if (!currentConfig.widgetsConfig) currentConfig.widgetsConfig = widgetsConfig();
        if (!Array.isArray(currentConfig.widgetsConfig.custom)) currentConfig.widgetsConfig.custom = [];
        return currentConfig.widgetsConfig.custom;
    }

    function defaultCustomWidget(index) {
        return {
            id: `custom-${index}`,
            name: `Custom Alert ${index}`,
            enabled: true,
            activation: { eventType: 'bits', minAmount: 25, rewardName: '' },
            appearance: defaultAppearance(520, true),
            html: '<div id="alert" class="alert"><div class="kicker">RAIKIRI ALERT</div><strong id="title">Custom support</strong><span id="message">Waiting for activation...</span></div>',
            css: `.alert { opacity: 0; transform: translateY(18px) scale(.96); padding: 18px 20px; border-radius: var(--widget-radius); background: rgba(12,12,16,var(--widget-bg-alpha)); border: 1px solid color-mix(in srgb, var(--widget-accent) 55%, white 12%); box-shadow: 0 18px 42px rgba(0,0,0,.32), 0 0 24px color-mix(in srgb, var(--widget-accent) 35%, transparent); transition: opacity .22s ease, transform .22s ease; }
.alert.show { opacity: 1; transform: translateY(0) scale(1); }
.kicker { color: var(--widget-accent); font-size: 12px; font-weight: 900; letter-spacing: .08em; }
#title { display: block; margin-top: 4px; font-size: 28px; line-height: 1.05; }
#message { display: block; margin-top: 6px; color: rgba(255,255,255,.78); font-size: 15px; }`,
            js: `const alertBox = document.getElementById('alert');
const title = document.getElementById('title');
const message = document.getElementById('message');
let hideTimer;
function beep() {
  const AudioContext = window.AudioContext || window.webkitAudioContext;
  if (!AudioContext) return;
  const ctx = new AudioContext();
  const osc = ctx.createOscillator();
  const gain = ctx.createGain();
  osc.type = 'sine';
  osc.frequency.value = 740;
  gain.gain.setValueAtTime(0.0001, ctx.currentTime);
  gain.gain.exponentialRampToValueAtTime(0.12, ctx.currentTime + 0.03);
  gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.42);
  osc.connect(gain);
  gain.connect(ctx.destination);
  osc.start();
  osc.stop(ctx.currentTime + 0.45);
}
window.addEventListener('raikiri:event', event => {
  const evt = event.detail;
  const amount = evt.amount ? ' (' + evt.amount + (evt.currency ? ' ' + evt.currency : '') + ')' : '';
  title.textContent = (evt.user || 'Viewer') + ' triggered ' + evt.type + amount;
  message.textContent = evt.rewardName || evt.message || 'Custom widget activated';
  alertBox.classList.add('show');
  beep();
  clearTimeout(hideTimer);
  hideTimer = setTimeout(() => alertBox.classList.remove('show'), 5000);
});`
        };
    }

    function loadCustomWidgetPicker() {
        const select = document.getElementById('widget-custom-select');
        if (!select) return;
        const widgets = ensureCustomWidgets();
        select.innerHTML = '';
        widgets.forEach((widget, index) => {
            const option = document.createElement('option');
            option.value = String(index);
            option.textContent = widget.name || widget.id || `Custom Widget ${index + 1}`;
            select.appendChild(option);
        });
        currentCustomWidgetIndex = Math.min(currentCustomWidgetIndex, Math.max(0, widgets.length - 1));
        select.value = String(currentCustomWidgetIndex);
    }

    function loadCurrentCustomWidgetUI() {
        const widgets = ensureCustomWidgets();
        const widget = widgets[currentCustomWidgetIndex];
        const disabled = !widget;
        ['widget-custom-name', 'widget-custom-id', 'widget-custom-activationEventType', 'widget-custom-activationMinAmount', 'widget-custom-activationRewardName', 'widget-custom-html', 'widget-custom-css', 'widget-custom-js'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.disabled = disabled;
        });
        if (!widget) {
            setInput('widget-custom-name', '');
            setInput('widget-custom-id', '');
            setInput('widget-custom-activationEventType', 'any');
            setInput('widget-custom-activationMinAmount', 0);
            setInput('widget-custom-activationRewardName', '');
            setInput('widget-custom-html', '');
            setInput('widget-custom-css', '');
            setInput('widget-custom-js', '');
            updateCustomWidgetURL('');
            return;
        }
        setInput('widget-custom-name', widget.name);
        setInput('widget-custom-id', widget.id);
        document.getElementById('widget-custom-enabled').checked = widget.enabled !== false;
        const activation = { eventType: 'any', minAmount: 0, rewardName: '', ...(widget.activation || {}) };
        setInput('widget-custom-activationEventType', activation.eventType);
        setInput('widget-custom-activationMinAmount', activation.minAmount);
        setInput('widget-custom-activationRewardName', activation.rewardName);
        setInput('widget-custom-html', widget.html);
        setInput('widget-custom-css', widget.css);
        setInput('widget-custom-js', widget.js);
        loadAppearanceUI('widget-custom', widget.appearance, defaultAppearance(520, true));
        updateCustomWidgetURL(widget.id);
    }

    function commitCurrentCustomWidget() {
        const widgets = ensureCustomWidgets();
        const widget = widgets[currentCustomWidgetIndex];
        if (!widget) return;
        widget.name = getInput('widget-custom-name', widget.name || 'Custom Widget');
        widget.id = slugify(getInput('widget-custom-id', widget.id || widget.name));
        widget.enabled = document.getElementById('widget-custom-enabled')?.checked !== false;
        widget.activation = {
            eventType: getInput('widget-custom-activationEventType', widget.activation?.eventType || 'any'),
            minAmount: Math.max(0, Number(getInput('widget-custom-activationMinAmount', widget.activation?.minAmount || 0)) || 0),
            rewardName: getInput('widget-custom-activationRewardName', widget.activation?.rewardName || '')
        };
        widget.html = getInput('widget-custom-html', widget.html || '');
        widget.css = getInput('widget-custom-css', widget.css || '');
        widget.js = getInput('widget-custom-js', widget.js || '');
        widget.appearance = readAppearanceUI('widget-custom', widget.appearance, defaultAppearance(520, true));
        updateCustomWidgetURL(widget.id);
        loadCustomWidgetPicker();
    }

    function slugify(value) {
        return String(value || 'custom-widget').toLowerCase().trim().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'custom-widget';
    }

    function customWidgetURL() {
        commitCurrentCustomWidget();
        const widgets = ensureCustomWidgets();
        const id = widgets[currentCustomWidgetIndex]?.id || '';
        return `http://localhost:30001/overlay/widgets/custom/?id=${encodeURIComponent(id)}`;
    }

    function updateCustomWidgetURL(id) {
        const el = document.getElementById('widget-custom-url');
        if (el) el.textContent = `http://localhost:30001/overlay/widgets/custom/?id=${encodeURIComponent(id || '')}`;
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
