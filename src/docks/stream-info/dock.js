const $ = (id) => document.getElementById(id);

const form = $('form');
const titleEl = $('title');
const countEl = $('count');
const gameChip = $('game-chip');
const gameInput = $('game-input');
const gameResults = $('game-results');
const applyBtn = $('apply');
const resultEl = $('result');
const targets = [...document.querySelectorAll('.target')];

const TITLE_LIMIT = { twitch: 140, youtube: 100 };
const TAG_LIMIT = 10;
const TAG_LENGTH = 25;
const TARGETS_KEY = 'raikiri.streamInfo.targets';

let game = null;            // { id, name, boxArtUrl }
let tags = [];
let baseline = null;        // field values last set by the dock itself, to detect local edits
let lastLoad = 0;
let pendingTimer = null;

// ---------- platform toggles ----------

function readTargets() {
    try {
        return { twitch: true, youtube: true, ...JSON.parse(localStorage.getItem(TARGETS_KEY) || '{}') };
    } catch {
        return { twitch: true, youtube: true };
    }
}

function enabled(platform) {
    return targets.find(t => t.dataset.platform === platform).getAttribute('aria-pressed') === 'true';
}

targets.forEach(btn => {
    btn.setAttribute('aria-pressed', String(readTargets()[btn.dataset.platform]));
    btn.addEventListener('click', () => {
        btn.setAttribute('aria-pressed', String(!enabled(btn.dataset.platform)));
        try {
            localStorage.setItem(TARGETS_KEY, JSON.stringify({ twitch: enabled('twitch'), youtube: enabled('youtube') }));
        } catch { /* storage unavailable */ }
        updateCount();
    });
});

function setState(platform, text, tone = '', tooltip = '') {
    const el = document.querySelector(`[data-state="${platform}"]`);
    el.textContent = text;
    el.className = 'target-state' + (tone ? ' ' + tone : '');
    el.closest('.target').title = tooltip;
}

function renderStates(info) {
    const tw = info.twitch;
    if (!tw.ok) {
        setState('twitch', tw.code === 'not_connected' ? 'desconectado' : 'error', 'bad', tw.error || '');
    } else if (!tw.canEdit) {
        setState('twitch', 'falta permiso', 'warn', 'Vuelve a autenticar Twitch en el dashboard de Raikiri');
    } else {
        setState('twitch', tw.live ? 'en vivo' : 'offline', tw.live ? 'live' : '', tw.title);
    }

    const yt = info.youtube;
    if (!yt.ok) {
        setState('youtube', yt.code === 'not_connected' ? 'desconectado' : 'error', 'bad', yt.error || '');
    } else if (yt.pending) {
        setState('youtube', 'al iniciar stream', 'warn', yt.pending);
    } else if (yt.broadcast?.status === 'live') {
        setState('youtube', 'en vivo', 'live', yt.broadcast.title);
    } else if (yt.broadcast) {
        setState('youtube', 'programado', '', yt.broadcast.title);
    } else {
        setState('youtube', 'sin transmisión', '', '');
    }
}

// ---------- title ----------

function titleLimit() {
    return enabled('youtube') ? TITLE_LIMIT.youtube : TITLE_LIMIT.twitch;
}

function cleanTitle() {
    return titleEl.value.replace(/\s+/g, ' ').trim();
}

function updateCount() {
    const n = [...cleanTitle()].length;
    const limit = titleLimit();
    countEl.textContent = `${n}/${limit}`;
    countEl.classList.toggle('over', n > limit);
    applyBtn.disabled = n > limit || (!n && !game && !tagsChanged()) || (!enabled('twitch') && !enabled('youtube'));
}

function autosize() {
    titleEl.style.height = 'auto';
    titleEl.style.height = titleEl.scrollHeight + 2 + 'px';
}

titleEl.addEventListener('input', () => { autosize(); updateCount(); });
titleEl.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
        e.preventDefault();
        form.requestSubmit();
    }
});

// ---------- game picker ----------

function setGame(next) {
    game = next ? { id: next.id, name: next.name, boxArtUrl: next.boxArtUrl || '' } : null;
    closeResults();
    gameInput.value = '';
    if (game) {
        $('game-art').src = game.boxArtUrl || '';
        $('game-art').style.visibility = game.boxArtUrl ? '' : 'hidden';
        $('game-name').textContent = game.name;
        gameChip.hidden = false;
        gameInput.hidden = true;
    } else {
        gameChip.hidden = true;
        gameInput.hidden = false;
    }
    updateCount();
}

$('game-edit').addEventListener('click', () => {
    gameChip.hidden = true;
    gameInput.hidden = false;
    gameInput.value = game?.name || '';
    gameInput.focus();
    gameInput.select();
    search(gameInput.value);
});

let searchTimer = null;
let searchSeq = 0;
let results = [];
let active = -1;

gameInput.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => search(gameInput.value), 180);
});

gameInput.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        if (!results.length) return;
        active = (active + (e.key === 'ArrowDown' ? 1 : -1) + results.length) % results.length;
        highlight();
    } else if (e.key === 'Enter') {
        e.preventDefault();
        if (results.length) setGame(results[Math.max(active, 0)]);
    } else if (e.key === 'Escape') {
        e.preventDefault();
        setGame(game);
        titleEl.focus();
    }
});

gameInput.addEventListener('blur', () => {
    // Leaving the search without picking restores the current game.
    setTimeout(() => {
        if (document.activeElement !== gameInput) setGame(game);
    }, 120);
});

async function search(query) {
    query = query.trim();
    const seq = ++searchSeq;
    if (!query) {
        closeResults();
        return;
    }
    let data;
    try {
        const res = await fetch('/api/stream-info/games?q=' + encodeURIComponent(query));
        data = await res.json();
        if (!res.ok) throw new Error(data.code === 'not_connected' ? 'Twitch no está conectado' : (data.error || res.statusText));
    } catch (err) {
        if (seq === searchSeq) showNote(err.message);
        return;
    }
    if (seq !== searchSeq) return;
    results = data;
    active = results.length ? 0 : -1;
    if (!results.length) {
        showNote('Sin resultados');
        return;
    }
    gameResults.replaceChildren(...results.map((g, i) => {
        const li = document.createElement('li');
        li.setAttribute('role', 'option');
        const img = document.createElement('img');
        img.src = g.boxArtUrl;
        img.alt = '';
        const name = document.createElement('span');
        name.textContent = g.name;
        li.append(img, name);
        li.addEventListener('mousedown', (e) => e.preventDefault());
        li.addEventListener('click', () => setGame(g));
        li.addEventListener('mousemove', () => { active = i; highlight(); });
        return li;
    }));
    openResults();
    highlight();
}

function showNote(text) {
    results = [];
    const li = document.createElement('li');
    li.className = 'note';
    li.textContent = text;
    gameResults.replaceChildren(li);
    openResults();
}

function highlight() {
    [...gameResults.children].forEach((li, i) => li.setAttribute('aria-selected', String(i === active)));
    gameResults.children[active]?.scrollIntoView({ block: 'nearest' });
}

function openResults() {
    gameResults.hidden = false;
    gameInput.setAttribute('aria-expanded', 'true');
}

function closeResults() {
    searchSeq++;
    results = [];
    active = -1;
    gameResults.hidden = true;
    gameInput.setAttribute('aria-expanded', 'false');
}

// ---------- tags ----------

const tagsEl = $('tags');
const tagInput = $('tag-input');

// Same rules as Twitch (and the server): letters and digits only, 25 chars, 10 tags.
function cleanTag(raw) {
    return raw.replace(/[^\p{L}\p{N}]/gu, '').slice(0, TAG_LENGTH);
}

function hasTag(tag) {
    return tags.some(t => t.toLowerCase() === tag.toLowerCase());
}

function tagChip(tag, onClick, label) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'tag';
    btn.textContent = tag;
    if (label) btn.setAttribute('aria-label', label);
    btn.addEventListener('click', onClick);
    return btn;
}

function setTags(next) {
    tags = [];
    next.forEach(t => {
        t = cleanTag(t);
        if (t && !hasTag(t) && tags.length < TAG_LIMIT) tags.push(t);
    });
    // Chips go before the input without moving it, so typing several tags keeps focus.
    tagsEl.querySelectorAll('.tag').forEach(el => el.remove());
    tagInput.before(...tags.map(t => {
        const chip = tagChip(t, () => {
            setTags(tags.filter(x => x !== t));
            tagInput.focus();
        }, `Quitar ${t}`);
        chip.classList.add('removable');
        return chip;
    }));
    tagInput.hidden = tags.length >= TAG_LIMIT;
    $('tag-count').textContent = `${tags.length}/${TAG_LIMIT}`;
    renderIdeaTags();
    updateCount();
}

function addTypedTag() {
    const tag = cleanTag(tagInput.value);
    tagInput.value = '';
    if (tag) setTags([...tags, tag]);
}

tagsEl.addEventListener('click', (e) => { if (e.target === tagsEl) tagInput.focus(); });
tagInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ',' || e.key === ' ') {
        e.preventDefault();
        addTypedTag();
    } else if (e.key === 'Backspace' && !tagInput.value && tags.length) {
        setTags(tags.slice(0, -1));
    }
});
tagInput.addEventListener('blur', addTypedTag);

function sameTags(a, b) {
    return a.length === b.length && a.every((t, i) => t === b[i]);
}

// ---------- suggestions ----------

const suggestBtn = $('suggest');
const notesEl = $('notes');
let ideaTags = [];

function renderIdeaTags() {
    $('idea-tags').replaceChildren(...ideaTags.map(t => {
        const chip = tagChip(t, () => setTags(hasTag(t) ? tags.filter(x => x.toLowerCase() !== t.toLowerCase()) : [...tags, t]));
        chip.setAttribute('aria-pressed', String(hasTag(t)));
        return chip;
    }));
    $('idea-tags-row').hidden = !ideaTags.length;
    $('use-tags').disabled = sameTags(tags, ideaTags);
}

$('use-tags').addEventListener('click', () => setTags(ideaTags));

function renderIdeas(data) {
    $('idea-titles').replaceChildren(...data.titles.map(title => {
        const li = document.createElement('li');
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.textContent = title;
        btn.addEventListener('click', () => {
            titleEl.value = title;
            autosize();
            updateCount();
            titleEl.focus();
        });
        li.append(btn);
        return li;
    }));
    ideaTags = data.tags || [];
    renderIdeaTags();
    $('ideas').hidden = false;
}

async function suggest() {
    if (suggestBtn.disabled) return;
    suggestBtn.disabled = true;
    suggestBtn.textContent = 'Pensando…';
    $('ideas-error').hidden = true;
    try {
        const res = await fetch('/api/stream-info/suggest', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ title: cleanTitle(), game, tags, notes: notesEl.value }),
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || res.statusText);
        renderIdeas(data);
    } catch (err) {
        const el = $('ideas-error');
        el.replaceChildren(Object.assign(document.createElement('span'), { className: 'bad', textContent: err.message }));
        el.hidden = false;
    } finally {
        suggestBtn.disabled = false;
        suggestBtn.textContent = 'Sugerir';
    }
}

suggestBtn.addEventListener('click', suggest);
notesEl.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
        e.preventDefault();
        suggest();
    }
});

// ---------- recent ----------

function renderRecent(recent) {
    $('recent-section').hidden = !recent.length;
    $('recent').replaceChildren(...recent.map(preset => {
        const li = document.createElement('li');
        const btn = document.createElement('button');
        btn.type = 'button';
        const img = document.createElement('img');
        img.alt = '';
        if (preset.game?.boxArtUrl) img.src = preset.game.boxArtUrl;
        else img.style.visibility = 'hidden';
        const text = document.createElement('span');
        text.className = 'text';
        const t = document.createElement('span');
        t.textContent = preset.title;
        text.append(t);
        if (preset.game) {
            const g = document.createElement('span');
            g.className = 'game-label';
            g.textContent = preset.game.name;
            text.append(g);
        }
        btn.append(img, text);
        btn.title = preset.title;
        btn.addEventListener('click', () => {
            titleEl.value = preset.title;
            autosize();
            if (preset.game) setGame(preset.game);
            if (preset.tags) setTags(preset.tags);
            updateCount();
            titleEl.focus();
        });
        li.append(btn);
        return li;
    }));
}

// ---------- load / apply ----------

function tagsChanged() {
    return baseline ? !sameTags(tags, baseline.tags) : tags.length > 0;
}

function edited() {
    if (!gameInput.hidden && gameInput.value) return true;
    if (tagInput.value) return true;
    return baseline && (titleEl.value !== baseline.title || (game?.id || '') !== (baseline.gameId || '') || tagsChanged());
}

function markBaseline() {
    baseline = { title: titleEl.value, gameId: game?.id || '', tags: [...tags] };
}

async function load({ fill = true } = {}) {
    lastLoad = Date.now();
    let info;
    try {
        info = await (await fetch('/api/stream-info')).json();
    } catch (err) {
        setState('twitch', 'Raikiri no responde', 'bad');
        setState('youtube', '', '');
        return;
    }
    renderStates(info);
    renderRecent(info.recent || []);

    const ytTitle = info.youtube.pending || info.youtube.broadcast?.title || '';
    const current = {
        // Show the title of the platform being targeted, so a YouTube-only change isn't overwritten.
        title: (enabled('twitch') ? info.twitch.title || ytTitle : ytTitle || info.twitch.title) || '',
        game: info.twitch.game || null,
    };
    // Only overwrite the fields while the user hasn't started editing them.
    if (fill && (!baseline || !edited())) {
        titleEl.value = current.title;
        setGame(current.game);
        setTags(info.twitch.tags || []);
        autosize();
        markBaseline();
    }
    updateCount();

    clearTimeout(pendingTimer);
    if (info.youtube.pending) pendingTimer = setTimeout(load, 30000);
}

const RESULT_TEXT = {
    not_connected: 'conéctalo en el dashboard',
    missing_scope: 'vuelve a autenticar en el dashboard',
};

function describe(platform, res) {
    const name = platform === 'twitch' ? 'Twitch' : 'YouTube';
    if (!res.ok) return [`${name}: ${RESULT_TEXT[res.code] || res.error}`, 'bad'];
    if (res.state === 'pending') return [`${name}: se aplica al iniciar el stream`, 'warn'];
    if (res.state === 'skipped') return [`${name}: sin cambios`, ''];
    return [`${name} ✓`, 'ok'];
}

let resultTimer = null;

form.addEventListener('submit', async (e) => {
    e.preventDefault();
    if (applyBtn.disabled) return;
    applyBtn.disabled = true;
    applyBtn.textContent = 'Actualizando…';
    resultEl.replaceChildren();
    clearTimeout(resultTimer);
    try {
        const res = await fetch('/api/stream-info', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                title: cleanTitle(),
                game,
                // Only send tags when they changed, so an unconnected Twitch can't wipe YouTube's.
                ...(tagsChanged() ? { tags } : {}),
                twitch: enabled('twitch'),
                youtube: enabled('youtube'),
            }),
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        const parts = ['twitch', 'youtube'].filter(p => data[p]).map(p => describe(p, data[p]));
        parts.forEach(([text, tone], i) => {
            if (i) resultEl.append(' · ');
            const span = document.createElement('span');
            span.className = tone;
            span.textContent = text;
            resultEl.append(span);
        });
        if (parts.every(([, tone]) => tone === 'ok')) {
            resultTimer = setTimeout(() => resultEl.replaceChildren(), 6000);
        }
        // What was just applied becomes the new unedited state of the fields.
        markBaseline();
        await load({ fill: false });
    } catch (err) {
        const span = document.createElement('span');
        span.className = 'bad';
        span.textContent = err.message;
        resultEl.replaceChildren(span);
    } finally {
        applyBtn.textContent = 'Actualizar';
        updateCount();
    }
});

// OBS keeps docks alive for the whole session; refresh when the dock gets attention again.
window.addEventListener('focus', () => {
    if (Date.now() - lastLoad > 20000) load();
});

setTags([]);
load();
