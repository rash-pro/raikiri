import { connectEvents } from "/shared/ws-client.js";
import { applyWidgetAppearance } from "/shared/widget-runtime.js";

const root = document.getElementById('events');

const labels = {
    superchat: 'Super Chat',
    supersticker: 'Super Sticker',
    membership: 'Membership',
    subscription: 'Subscription',
    bits: 'Bits',
    raid: 'Raid',
    gift: 'Gifted Subs',
    follow: 'Follow'
};

const icons = {
    superchat: '$',
    supersticker: '$',
    membership: 'M',
    subscription: 'S',
    bits: 'B',
    raid: 'R',
    gift: 'G',
    follow: '+'
};

function render(state) {
    const config = state?.recentEvents;
    applyWidgetAppearance(state?.config?.recentEvents?.appearance || {});
    root.innerHTML = '';
    if (!config) return;
    config.forEach(evt => {
        const item = document.createElement('article');
        item.className = `event event-${evt.type}`;
        const label = labels[evt.type] || evt.type;
        item.innerHTML = `
            <div class="event-icon">${icons[evt.type] || '*'}</div>
            <div class="event-main">
                <div class="event-user">${evt.user || evt.platform || 'Viewer'}</div>
                <div class="event-label">${label}</div>
            </div>
            <div class="event-amount">${evt.amount || ''}</div>
        `;
        root.appendChild(item);
    });
}

fetch('/api/widgets/state')
    .then(res => res.json())
    .then(render)
    .catch(console.error);

connectEvents('/ws/widgets', {
    state: render,
    connect: () => console.log('Connected to Recent Events widget')
});
