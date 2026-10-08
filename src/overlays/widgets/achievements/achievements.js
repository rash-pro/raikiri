import { connectEvents } from "/shared/ws-client.js";
import { widgetParams } from "/shared/widget-runtime.js";

// URL params: ?lang=es|en  ?theme=ff8|skyline|outrun  ?pos=top-center|top-left|top-right|bottom-*
//             ?hold=6 (seconds)  ?x= ?y= (margins px)  ?scale=1  ?sound=0  ?volume=0.5  ?demo=1
const params = widgetParams();

const STEAM_LANG = { es: 'spanish', en: 'english', fr: 'french', de: 'german', it: 'italian', ja: 'japanese', pt: 'portuguese', ko: 'koreana', ru: 'russian' };
const LABELS = {
    spanish: { unlocked: 'Logro desbloqueado', generic: 'Logro desbloqueado', hidden: 'Logro secreto' },
    english: { unlocked: 'Achievement unlocked', generic: 'Achievement unlocked', hidden: 'Secret achievement' }
};

const langParam = (params.get('lang') || 'es').toLowerCase();
const LANG = STEAM_LANG[langParam] || langParam;
const LABEL = LABELS[LANG] || LABELS.english;
const HOLD_MS = seconds('hold', 6) * 1000;
const GAP_MS = 450;
const OPEN_MS = 340;
const CLOSE_MS = 260;
const SOUND = params.get('sound') !== '0';
const VOLUME = clamp(Number(params.get('volume') ?? 0.5), 0, 1);
const THEME = (params.get('theme') || 'ff8').replace(/[^a-z0-9-]/gi, '') || 'ff8';
const POS = /^(top|bottom)-(left|center|right)$/.test(params.get('pos') || '') ? params.get('pos') : 'top-center';

const stage = document.getElementById('stage');
const toast = document.getElementById('toast');
const iconEl = document.getElementById('icon');
const labelEl = document.getElementById('label');
const nameEl = document.getElementById('name');
const descEl = document.getElementById('desc');
const countEl = document.getElementById('count');
const gameEl = document.getElementById('game');
const chime = document.getElementById('chime');

document.documentElement.className = `theme-${THEME}`;
stage.dataset.pos = POS;
document.documentElement.style.setProperty('--scale', String(Number(params.get('scale')) || 1));
if (params.has('x')) document.documentElement.style.setProperty('--margin-x', `${Number(params.get('x')) || 0}px`);
if (params.has('y')) document.documentElement.style.setProperty('--margin-y', `${Number(params.get('y')) || 0}px`);
chime.volume = VOLUME;

const queue = [];
let busy = false;
let timer = 0;

function seconds(key, fallback) {
    const value = Number(params.get(key));
    return Number.isFinite(value) && value > 0 ? value : fallback;
}

function clamp(value, min, max) {
    return Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : min;
}

function text(map, fallback = '') {
    if (!map) return fallback;
    return (map[LANG] || map.english || Object.values(map).find(Boolean) || fallback).trim();
}

function enqueue(event) {
    if (!event) return;
    queue.push(event);
    if (!busy) next();
}

function next() {
    const event = queue.shift();
    if (!event) {
        busy = false;
        return;
    }
    busy = true;
    render(event);
    toast.hidden = false;
    toast.classList.remove('hide', 'show');
    void toast.offsetWidth;
    toast.classList.add('show');
    playChime();
    clearTimeout(timer);
    timer = setTimeout(close, OPEN_MS + HOLD_MS);
}

function close() {
    toast.classList.remove('show');
    toast.classList.add('hide');
    timer = setTimeout(() => {
        toast.hidden = true;
        toast.classList.remove('hide');
        timer = setTimeout(next, GAP_MS);
    }, CLOSE_MS);
}

function render(event) {
    const generic = event.generic || !event.apiName;
    const name = generic ? LABEL.generic : text(event.names, event.apiName);
    labelEl.textContent = generic ? (event.game || '') : (event.hidden ? LABEL.hidden : LABEL.unlocked);
    nameEl.textContent = name;
    descEl.textContent = generic ? '' : text(event.descs);
    gameEl.textContent = generic ? '' : (event.game || '');
    if (event.total > 0) {
        countEl.replaceChildren(document.createTextNode(String(event.unlocked)), small(`/${event.total}`));
    } else {
        countEl.textContent = '';
    }
    if (event.icon) {
        iconEl.src = event.icon;
    } else {
        iconEl.removeAttribute('src');
    }
    // Preload the next icon so the window never opens on a blank frame.
    if (queue[0]?.icon) new Image().src = queue[0].icon;
}

function small(value) {
    const el = document.createElement('small');
    el.textContent = value;
    return el;
}

function playChime() {
    if (!SOUND) return;
    try {
        chime.currentTime = 0;
        chime.play().catch(() => {});
    } catch { /* autoplay blocked outside OBS */ }
}

const DEMO = {
    appid: 39150, game: 'FINAL FANTASY VIII', apiName: 'UNLOCK_GF_SHIVA',
    names: { english: 'Shiva', spanish: 'Shiva' },
    descs: { english: 'Unlock Guardian Force Shiva', spanish: 'Desbloquea al Guardián de la Fuerza Shiva' },
    icon: 'https://shared.steamstatic.com/community_assets/images/apps/39150/d661c18c7dc774c8b9a01905d674420eb2050c94.jpg',
    hidden: false, unlocked: 11, total: 45, time: Math.floor(Date.now() / 1000), source: 'demo'
};

connectEvents('/ws/widgets', {
    achievement_unlocked: enqueue,
    connect: () => console.log('Connected to achievements widget')
});

if (params.get('demo')) {
    await document.fonts.ready;
    setTimeout(() => enqueue({ ...DEMO }), 400);
}

window.addEventListener('keydown', event => {
    if (event.code === 'Space' || event.code === 'Enter') enqueue({ ...DEMO });
});
