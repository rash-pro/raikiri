import { connectEvents } from "/shared/ws-client.js";
import { widgetParams } from "/shared/widget-runtime.js";

// Mainline series. Remove entries per scene with ?exclude=11,14
const ALL_GAMES = [
    { n: 1, numeral: 'I', year: 1987 },
    { n: 2, numeral: 'II', year: 1988 },
    { n: 3, numeral: 'III', year: 1990 },
    { n: 4, numeral: 'IV', year: 1991 },
    { n: 5, numeral: 'V', year: 1992 },
    { n: 6, numeral: 'VI', year: 1994 },
    { n: 7, numeral: 'VII', year: 1997 },
    { n: 8, numeral: 'VIII', year: 1999 },
    { n: 9, numeral: 'IX', year: 2000 },
    { n: 10, numeral: 'X', year: 2001 },
    { n: 11, numeral: 'XI', year: 2002 },
    { n: 12, numeral: 'XII', year: 2006 },
    { n: 13, numeral: 'XIII', year: 2009 },
    { n: 14, numeral: 'XIV', year: 2010 },
    { n: 15, numeral: 'XV', year: 2016 },
    { n: 16, numeral: 'XVI', year: 2023 }
];

const INK = '#07030f';
const PALETTE = ['#ff4fd8', '#37f3ff', '#ffd36b', '#b58cff'];

const params = widgetParams();
const excluded = new Set((params.get('exclude') || '').split(',').map(Number).filter(Boolean));
const GAMES = ALL_GAMES.filter(game => !excluded.has(game.n));
const COLORS = GAMES.map((_, i) => PALETTE[i % PALETTE.length]);
if (GAMES.length > 1 && COLORS[GAMES.length - 1] === COLORS[0]) COLORS[GAMES.length - 1] = PALETTE[2];

const SPIN_MS = seconds('spin', 7) * 1000;
const HOLD_MS = seconds('hold', 9) * 1000;
const BOOT_MS = 600;
const OUT_MS = 700;
const SOUND = params.get('sound') !== '0';

const stage = document.getElementById('stage');
const titleEl = document.getElementById('title');
const resultEl = document.getElementById('result');
const resultNumber = document.getElementById('result-number');
const resultYear = document.getElementById('result-year');
const wheel = document.getElementById('wheel');
const ctx = wheel.getContext('2d');
const fx = document.getElementById('fx');
const fxCtx = fx.getContext('2d');

titleEl.textContent = params.get('title') ?? '¿Qué Final Fantasy jugamos?';
stage.style.setProperty('--scale', String(Number(params.get('scale')) || 1));

const CX = 190;
const CY = 210;
const R_RIM = 186;
const R_SEG = 166;
const R_HUB = 30;
const LABEL_R = 138;
const BULBS = 24;
const SEG = (Math.PI * 2) / GAMES.length;

let state = 'idle';
let angle = 0;
let pointerKick = 0;
let pointerVel = 0;
let spin = null;
let winner = -1;
let resultAt = 0;
let lastSegment = 0;
let lastTickAt = 0;
let particles = [];
let rafId = 0;
let lastFrame = 0;
let holdTimer = 0;
let outTimer = 0;

function seconds(key, fallback) {
    const value = Number(params.get(key));
    return Number.isFinite(value) && value > 0 ? value : fallback;
}

function randomUnit() {
    const buf = new Uint32Array(1);
    crypto.getRandomValues(buf);
    return buf[0] / 2 ** 32;
}

// Index of the segment under the top pointer for a given wheel rotation.
function segmentAt(rotation) {
    const local = ((-rotation % (Math.PI * 2)) + Math.PI * 2) % (Math.PI * 2);
    return Math.floor(local / SEG) % GAMES.length;
}

function trigger() {
    if (!GAMES.length || state === 'spinning' || state === 'booting') return;
    clearTimeout(holdTimer);
    clearTimeout(outTimer);
    if (state === 'result') {
        hideResult();
        startSpin();
        return;
    }
    stage.classList.remove('leaving');
    stage.hidden = false;
    stage.classList.remove('entering');
    void stage.offsetWidth;
    stage.classList.add('entering');
    state = 'booting';
    winner = -1;
    startLoop();
    sfxBoot();
    setTimeout(() => {
        stage.classList.remove('entering');
        startSpin();
    }, BOOT_MS);
}

function startSpin() {
    const target = Math.floor(randomUnit() * GAMES.length);
    const within = 0.15 + randomUnit() * 0.7;
    const turns = 6 + Math.floor(randomUnit() * 3);
    const base = angle + turns * Math.PI * 2;
    const wanted = Math.PI * 2 - (target + within) * SEG;
    const delta = ((wanted - base) % (Math.PI * 2) + Math.PI * 2) % (Math.PI * 2);
    spin = { from: angle, to: base + delta, start: performance.now(), target };
    lastSegment = segmentAt(angle);
    winner = -1;
    state = 'spinning';
}

function finishSpin() {
    angle = spin.to % (Math.PI * 2);
    winner = spin.target;
    spin = null;
    state = 'result';
    resultAt = performance.now();
    showResult(GAMES[winner], COLORS[winner]);
    sfxWin();
    holdTimer = setTimeout(leave, HOLD_MS);
}

function leave() {
    state = 'leaving';
    stage.classList.add('leaving');
    outTimer = setTimeout(() => {
        stage.hidden = true;
        stage.classList.remove('leaving');
        hideResult();
        state = 'idle';
        winner = -1;
    }, OUT_MS);
}

function showResult(game, color) {
    resultNumber.textContent = game.numeral;
    resultYear.textContent = String(game.year);
    resultEl.style.setProperty('--sign', color);
    resultEl.hidden = false;
    resultEl.classList.remove('show');
    void resultEl.offsetWidth;
    resultEl.classList.add('show');
    const rect = resultEl.getBoundingClientRect();
    burst((rect.left + rect.width / 2) / 2, (rect.top + rect.height / 2) / 2, color);
}

function hideResult() {
    resultEl.hidden = true;
    resultEl.classList.remove('show');
}

function easeOut(t) {
    return 1 - Math.pow(1 - t, 4);
}

function startLoop() {
    if (rafId) return;
    lastFrame = performance.now();
    rafId = requestAnimationFrame(frame);
}

function frame(now) {
    const dt = Math.min(0.05, (now - lastFrame) / 1000);
    lastFrame = now;

    if (state === 'spinning' && spin) {
        const t = Math.min(1, (now - spin.start) / SPIN_MS);
        angle = spin.from + (spin.to - spin.from) * easeOut(t);
        const segment = segmentAt(angle);
        if (segment !== lastSegment) {
            lastSegment = segment;
            pointerKick = -0.42;
            pointerVel = 0;
            if (now - lastTickAt > 28) {
                lastTickAt = now;
                sfxTick();
            }
        }
        if (t >= 1) finishSpin();
    }

    // Pointer springs back to rest after each peg hit.
    pointerVel += (-pointerKick * 180 - pointerVel * 16) * dt;
    pointerKick += pointerVel * dt;

    drawWheel(now);
    drawParticles(dt);

    if (state === 'idle' && !particles.length) {
        rafId = 0;
        ctx.clearRect(0, 0, wheel.width, wheel.height);
        fxCtx.clearRect(0, 0, fx.width, fx.height);
        return;
    }
    rafId = requestAnimationFrame(frame);
}

function drawWheel(now) {
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.clearRect(0, 0, wheel.width, wheel.height);
    ctx.setTransform(wheel.width / 380, 0, 0, wheel.width / 380, 0, 0);

    disc(R_RIM, INK);
    disc(R_RIM - 2, '#ff4fd8');
    disc(R_RIM - 5, '#140a3c');
    disc(R_SEG + 2, INK);

    drawBulbs(now);

    ctx.save();
    ctx.translate(CX, CY);
    ctx.rotate(angle);
    const flashing = state === 'result' && now - resultAt < 1300 && Math.floor((now - resultAt) / 110) % 2 === 0;
    for (let i = 0; i < GAMES.length; i++) {
        const a0 = -Math.PI / 2 + i * SEG;
        const a1 = a0 + SEG;
        ctx.beginPath();
        ctx.moveTo(0, 0);
        ctx.arc(0, 0, R_SEG, a0, a1);
        ctx.closePath();
        ctx.fillStyle = i === winner && flashing ? '#ffffff' : COLORS[i];
        ctx.fill();

        // Tangential numerals: the winner reads upright under the pointer.
        ctx.save();
        ctx.rotate(a0 + SEG / 2 + Math.PI / 2);
        ctx.fillStyle = INK;
        ctx.font = "700 22px 'Pixelify Sans', monospace";
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        const label = GAMES[i].numeral;
        const room = SEG * LABEL_R * 0.86;
        const width = ctx.measureText(label).width;
        if (width > room) ctx.scale(room / width, 1);
        ctx.fillText(label, 0, -LABEL_R);
        ctx.restore();

        if (winner >= 0 && i !== winner) {
            ctx.fillStyle = 'rgba(7, 3, 15, 0.6)';
            ctx.fill();
        }
    }
    ctx.strokeStyle = INK;
    ctx.lineWidth = 2;
    for (let i = 0; i < GAMES.length; i++) {
        const a = -Math.PI / 2 + i * SEG;
        ctx.beginPath();
        ctx.moveTo(0, 0);
        ctx.lineTo(Math.cos(a) * R_SEG, Math.sin(a) * R_SEG);
        ctx.stroke();
    }
    const shade = ctx.createRadialGradient(0, 0, R_HUB, 0, 0, R_SEG * 0.7);
    shade.addColorStop(0, 'rgba(7, 3, 15, 0.45)');
    shade.addColorStop(1, 'rgba(7, 3, 15, 0)');
    ctx.fillStyle = shade;
    ctx.beginPath();
    ctx.arc(0, 0, R_SEG, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();

    drawHub();
    drawPointer();
}

function disc(r, color) {
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(CX, CY, r, 0, Math.PI * 2);
    ctx.fill();
}

function drawBulbs(now) {
    const ringR = (R_RIM - 5 + R_SEG + 2) / 2;
    const chase = Math.floor(now / 70);
    const blink = Math.floor(now / 160) % 2 === 0;
    for (let i = 0; i < BULBS; i++) {
        let lit;
        if (state === 'spinning') lit = (i + chase) % 3 === 0;
        else if (state === 'result') lit = blink;
        else lit = (i + Math.floor(now / 400)) % 2 === 0;
        const a = (i / BULBS) * Math.PI * 2;
        const x = Math.round(CX + Math.cos(a) * ringR);
        const y = Math.round(CY + Math.sin(a) * ringR);
        ctx.fillStyle = lit ? '#fff4c2' : '#3a2470';
        ctx.fillRect(x - 3, y - 3, 6, 6);
        if (lit) {
            ctx.fillStyle = '#ffffff';
            ctx.fillRect(x - 2, y - 2, 2, 2);
        }
    }
}

function drawHub() {
    disc(R_HUB, INK);
    disc(R_HUB - 3, '#2b1464');
    ctx.save();
    ctx.translate(CX, CY);
    // Crystal
    ctx.beginPath();
    ctx.moveTo(0, -19);
    ctx.lineTo(11, -5);
    ctx.lineTo(0, 19);
    ctx.lineTo(-11, -5);
    ctx.closePath();
    ctx.fillStyle = INK;
    ctx.fill();
    ctx.beginPath();
    ctx.moveTo(0, -15);
    ctx.lineTo(8, -5);
    ctx.lineTo(0, 14);
    ctx.closePath();
    ctx.fillStyle = '#37f3ff';
    ctx.fill();
    ctx.beginPath();
    ctx.moveTo(0, -15);
    ctx.lineTo(-8, -5);
    ctx.lineTo(0, 14);
    ctx.closePath();
    ctx.fillStyle = '#1a8fb8';
    ctx.fill();
    ctx.fillStyle = '#e8fdff';
    ctx.fillRect(-1, -12, 2, 8);
    ctx.restore();
}

function drawPointer() {
    ctx.save();
    ctx.translate(CX, 8);
    ctx.rotate(pointerKick);
    ctx.beginPath();
    ctx.moveTo(-16, -2);
    ctx.lineTo(16, -2);
    ctx.lineTo(0, 42);
    ctx.closePath();
    ctx.fillStyle = INK;
    ctx.fill();
    ctx.beginPath();
    ctx.moveTo(-11, 1);
    ctx.lineTo(11, 1);
    ctx.lineTo(0, 33);
    ctx.closePath();
    ctx.fillStyle = '#ffd36b';
    ctx.fill();
    ctx.fillStyle = '#fff4c2';
    ctx.fillRect(-6, 3, 4, 4);
    ctx.restore();
}

function burst(x, y, color) {
    const colors = [color, '#ffffff', ...PALETTE];
    for (let i = 0; i < 90; i++) {
        const a = randomUnit() * Math.PI * 2;
        const speed = 60 + randomUnit() * 220;
        particles.push({
            x, y,
            vx: Math.cos(a) * speed,
            vy: Math.sin(a) * speed - 90,
            life: 1 + randomUnit() * 0.8,
            size: randomUnit() < 0.3 ? 3 : 2,
            color: colors[Math.floor(randomUnit() * colors.length)]
        });
    }
    startLoop();
}

function drawParticles(dt) {
    fxCtx.clearRect(0, 0, fx.width, fx.height);
    particles = particles.filter(p => (p.life -= dt) > 0);
    for (const p of particles) {
        p.vy += 260 * dt;
        p.vx *= 1 - 1.2 * dt;
        p.x += p.vx * dt;
        p.y += p.vy * dt;
        if (p.life < 0.3 && Math.floor(p.life * 30) % 2) continue;
        fxCtx.fillStyle = p.color;
        fxCtx.fillRect(Math.round(p.x), Math.round(p.y), p.size, p.size);
    }
}

let audio = null;

function audioCtx() {
    if (!SOUND) return null;
    audio ??= new AudioContext();
    if (audio.state === 'suspended') audio.resume();
    return audio;
}

function tone(freq, duration, { type = 'square', gain = 0.05, when = 0 } = {}) {
    const ac = audioCtx();
    if (!ac) return;
    const t = ac.currentTime + when;
    const osc = ac.createOscillator();
    const amp = ac.createGain();
    osc.type = type;
    osc.frequency.setValueAtTime(freq, t);
    amp.gain.setValueAtTime(gain, t);
    amp.gain.exponentialRampToValueAtTime(0.0001, t + duration);
    osc.connect(amp).connect(ac.destination);
    osc.start(t);
    osc.stop(t + duration + 0.02);
}

function sfxTick() {
    tone(1500, 0.03, { gain: 0.035 });
}

function sfxBoot() {
    tone(220, 0.08, { when: 0 });
    tone(440, 0.08, { when: 0.09 });
    tone(880, 0.12, { when: 0.18 });
}

function sfxWin() {
    [523.25, 659.25, 783.99, 1046.5].forEach((freq, i) => {
        tone(freq, i === 3 ? 0.6 : 0.12, { when: i * 0.1, gain: 0.06 });
    });
    tone(130.81, 0.7, { type: 'triangle', gain: 0.12, when: 0.3 });
}

await document.fonts.load("700 22px 'Pixelify Sans'").catch(() => {});

connectEvents('/ws/widgets', {
    roulette_spin: trigger,
    connect: () => console.log('Connected to Roulette widget')
});

window.addEventListener('keydown', event => {
    if (event.code === 'Space' || event.code === 'Enter') trigger();
});
